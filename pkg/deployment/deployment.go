package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yarlson/aerie/pkg/config"
	"io"
	"strings"
	"time"
)

const (
	newContainerSuffix = "_new"
)

type Executor interface {
	RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error)
}

type Deployment struct {
	executor Executor
}

func NewUpdater(executor Executor) *Deployment {
	return &Deployment{executor: executor}
}

func (d *Deployment) InstallService(service *config.Service, network string) error {
	if err := d.pullImage(service.Image); err != nil {
		return fmt.Errorf("failed to pull image for %s: %v", service.Image, err)
	}

	if err := d.startContainer(service, network, ""); err != nil {
		return fmt.Errorf("failed to start container for %s: %v", service.Image, err)
	}

	svcName := service.Name

	if err := d.performHealthChecks(svcName, service.HealthCheck); err != nil {
		return fmt.Errorf("install failed for %s: container is unhealthy: %w", svcName, err)
	}

	return nil
}

func (d *Deployment) UpdateService(service *config.Service, network string) error {
	svcName := service.Name

	if err := d.pullImage(service.Image); err != nil {
		return fmt.Errorf("failed to pull new image for %s: %v", svcName, err)
	}

	if err := d.startContainer(service, network, newContainerSuffix); err != nil {
		return fmt.Errorf("failed to start new container for %s: %v", svcName, err)
	}

	if err := d.performHealthChecks(svcName+newContainerSuffix, service.HealthCheck); err != nil {
		if _, err := d.runCommand(context.Background(), "docker", "rm", "-f", svcName+newContainerSuffix); err != nil {
			return fmt.Errorf("update failed for %s: new container is unhealthy and cleanup failed: %v", svcName, err)
		}
		return fmt.Errorf("update failed for %s: new container is unhealthy: %w", svcName, err)
	}

	if err := d.switchTraffic(svcName, network); err != nil {
		return fmt.Errorf("failed to switch traffic for %s: %v", svcName, err)
	}

	if err := d.cleanup(svcName, network); err != nil {
		return fmt.Errorf("failed to cleanup for %s: %v", svcName, err)
	}

	return nil
}

type containerInfo struct {
	ID     string
	Config struct {
		Image string
	}
	NetworkSettings struct {
		Networks map[string]struct{ Aliases []string }
	}
	HostConfig struct {
		Binds []string
	}
}

func (d *Deployment) getContainerID(service, network string) (string, error) {
	info, err := d.getContainerInfo(service, network)
	if err != nil {
		return "", err
	}

	return info.ID, err
}

func (d *Deployment) getContainerInfo(service, network string) (*containerInfo, error) {
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

func (d *Deployment) startContainer(service *config.Service, network, suffix string) error {
	svcName := service.Name

	args := []string{"run", "-d", "--name", svcName + suffix, "--network", network, "--network-alias", svcName + suffix}

	for _, env := range service.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", env.Name, env.Value))
	}

	for _, volume := range service.Volumes {
		args = append(args, "-v", volume)
	}

	if service.HealthCheck != nil {
		args = append(args, "--health-cmd", fmt.Sprintf("curl -f http://localhost:%d/%s || exit 1", service.Port, service.HealthCheck.Path))
		args = append(args, "--health-interval", fmt.Sprintf("%ds", int(service.HealthCheck.Interval.Seconds())))
		args = append(args, "--health-retries", fmt.Sprintf("%d", service.HealthCheck.Retries))
		args = append(args, "--health-timeout", fmt.Sprintf("%ds", int(service.HealthCheck.Timeout.Seconds())))
	}

	args = append(args, service.Image)

	_, err := d.runCommand(context.Background(), "docker", args...)
	return err
}

func (d *Deployment) performHealthChecks(container string, healthCheck *config.HealthCheck) error {
	for i := 0; i < healthCheck.Retries; i++ {
		output, err := d.runCommand(context.Background(), "docker", "inspect", "--format={{.State.Health.Status}}", container)
		if err == nil && strings.TrimSpace(output) == "healthy" {
			return nil
		}
		time.Sleep(healthCheck.Interval)
	}
	return fmt.Errorf("container failed to become healthy")
}

func (d *Deployment) switchTraffic(service, network string) error {
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

func (d *Deployment) cleanup(service, network string) error {
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

func (d *Deployment) pullImage(imageName string) error {
	_, err := d.runCommand(context.Background(), "docker", "pull", imageName)
	return err
}

// Helper function to run commands and read output
func (d *Deployment) runCommand(ctx context.Context, command string, args ...string) (string, error) {
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
