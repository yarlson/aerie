package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	healthCheckRetries  = 5
	healthCheckInterval = 10 * time.Second
	newContainerSuffix  = "_new"
)

type Executor interface {
	RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error)
}

type DockerUpdater struct {
	executor Executor
}

func NewDockerUpdater(executor Executor) *DockerUpdater {
	return &DockerUpdater{executor: executor}
}

func (d *DockerUpdater) UpdateService(service, network string) error {
	imageName, err := d.getImageName(service, network)
	if err != nil {
		return fmt.Errorf("failed to get image name for %s: %v", service, err)
	}

	if err := d.pullImage(imageName); err != nil {
		return fmt.Errorf("failed to pull new image for %s: %v", service, err)
	}

	if err := d.startNewContainer(service, network); err != nil {
		return fmt.Errorf("failed to start new container for %s: %v", service, err)
	}

	if err := d.performHealthChecks(service + newContainerSuffix); err != nil {
		if _, err := d.runCommand(context.Background(), "docker", "rm", "-f", service+newContainerSuffix); err != nil {
			return fmt.Errorf("update failed for %s: new container is unhealthy and cleanup failed: %v", service, err)
		}
		return fmt.Errorf("update failed for %s: new container is unhealthy: %w", service, err)
	}

	if err := d.switchTraffic(service, network); err != nil {
		return fmt.Errorf("failed to switch traffic for %s: %v", service, err)
	}

	if err := d.cleanup(service, network); err != nil {
		return fmt.Errorf("failed to cleanup for %s: %v", service, err)
	}

	return nil
}

type containerInfo struct {
	ID     string
	Config struct {
		Image  string
		Env    []string
		Labels map[string]string
	}
	NetworkSettings struct {
		Networks map[string]struct{ Aliases []string }
	}
	HostConfig struct {
		Binds []string
	}
}

func (d *DockerUpdater) getImageName(service, network string) (string, error) {
	info, err := d.getContainerInfo(service, network)
	if err != nil {
		return "", fmt.Errorf("failed to get container info: %w", err)
	}

	return info.Config.Image, nil
}

func (d *DockerUpdater) getContainerID(service, network string) (string, error) {
	info, err := d.getContainerInfo(service, network)
	if err != nil {
		return "", err
	}

	return info.ID, err
}

func (d *DockerUpdater) getContainerInfo(service, network string) (*containerInfo, error) {
	output, err := d.runCommand(context.Background(), "docker", "ps", "-q", "--filter", fmt.Sprintf("network=%s", network))
	if err != nil {
		return nil, fmt.Errorf("failed to get container IDs: %w", err)
	}

	containerIDs := strings.Fields(output)
	for _, cid := range containerIDs {
		inspectOutput, err := d.runCommand(context.Background(), "docker", "inspect", cid)
		if err != nil {
			continue
		}

		var containerInfos []containerInfo
		if err := json.Unmarshal([]byte(inspectOutput), &containerInfos); err != nil || len(containerInfos) == 0 {
			continue
		}

		if aliases, ok := containerInfos[0].NetworkSettings.Networks[network]; ok {
			for _, alias := range aliases.Aliases {
				if alias == service {
					return &containerInfos[0], nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no container found with alias %s in network %s", service, network)
}

func (d *DockerUpdater) startNewContainer(service, network string) error {
	imageName, err := d.getImageName(service, network)
	if err != nil {
		return fmt.Errorf("failed to get image name: %w", err)
	}

	info, err := d.getContainerInfo(service, network)
	if err != nil {
		return fmt.Errorf("failed to get container info: %w", err)
	}

	args := []string{"run", "-d", "--name", service + newContainerSuffix, "--network", network, "--network-alias", service + newContainerSuffix}

	for _, env := range info.Config.Env {
		args = append(args, "-e", env)
	}

	for _, bind := range info.HostConfig.Binds {
		args = append(args, "-v", bind)
	}

	for k, v := range info.Config.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args,
		"--health-cmd", "curl -f http://localhost:8080/health || exit 1",
		"--health-interval=5s",
		"--health-retries=3",
		"--health-timeout=2s",
		imageName,
	)

	_, err = d.runCommand(context.Background(), "docker", args...)
	return err
}

func (d *DockerUpdater) performHealthChecks(container string) error {
	for i := 0; i < healthCheckRetries; i++ {
		output, err := d.runCommand(context.Background(), "docker", "inspect", "--format={{.State.Health.Status}}", container)
		if err == nil && strings.TrimSpace(output) == "healthy" {
			return nil
		}
		time.Sleep(healthCheckInterval)
	}
	return fmt.Errorf("container failed to become healthy")
}

func (d *DockerUpdater) switchTraffic(service, network string) error {
	newContainer := service + newContainerSuffix
	oldContainer, err := d.getContainerID(service, network)
	if err != nil {
		return fmt.Errorf("failed to get old container ID: %v", err)
	}

	cmds := [][]string{
		{"docker", "network", "disconnect", network, newContainer},
		{"docker", "network", "connect", "--alias", service, network, newContainer},
		{"docker", "network", "disconnect", network, oldContainer},
	}

	for _, cmd := range cmds {
		if _, err := d.runCommand(context.Background(), cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("failed to execute command '%s': %v", strings.Join(cmd, " "), err)
		}
	}

	return nil
}

func (d *DockerUpdater) cleanup(service, network string) error {
	oldContainer, err := d.getContainerID(service, network)
	if err != nil {
		return fmt.Errorf("failed to get old container ID: %v", err)
	}

	cmds := [][]string{
		{"docker", "stop", oldContainer},
		{"docker", "rm", oldContainer},
		{"docker", "rename", service + newContainerSuffix, service},
	}

	for _, cmd := range cmds {
		if _, err := d.runCommand(context.Background(), cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("failed to execute command '%s': %v", strings.Join(cmd, " "), err)
		}
	}

	return nil
}

func (d *DockerUpdater) pullImage(imageName string) error {
	_, err := d.runCommand(context.Background(), "docker", "pull", imageName)
	return err
}

// Helper function to run commands and read output
func (d *DockerUpdater) runCommand(ctx context.Context, command string, args ...string) (string, error) {
	output, err := d.executor.RunCommand(ctx, command, args...)
	if err != nil {
		return "", err
	}

	var outputBuilder strings.Builder
	_, err = io.Copy(&outputBuilder, output)
	if err != nil {
		return "", fmt.Errorf("failed to read command output: %w", err)
	}

	return outputBuilder.String(), nil
}
