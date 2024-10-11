package deployment

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yarlson/aerie/pkg/ssh"
)

const (
	healthCheckRetries  = 5
	healthCheckInterval = 10 * time.Second
)

type SSHClient struct {
	*ssh.Client
}

type DockerUpdater struct {
	client *ssh.Client
}

func NewDockerUpdater(client *ssh.Client) *DockerUpdater {
	return &DockerUpdater{client: client}
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
	cmd := fmt.Sprintf("docker ps -q --filter network=%s", network)
	output, err := d.client.RunCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get container IDs: %w", err)
	}

	containerIDs := strings.Fields(string(output))
	for _, cid := range containerIDs {
		cmd := fmt.Sprintf("docker inspect %s", cid)
		inspectOutput, err := d.client.RunCommand(cmd)
		if err != nil {
			continue
		}

		var containerInfos []containerInfo
		if err := json.Unmarshal(inspectOutput, &containerInfos); err != nil || len(containerInfos) == 0 {
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

	envVars := strings.Join(info.Config.Env, " -e ")
	volumeBinds := strings.Join(info.HostConfig.Binds, " -v ")

	var labels []string
	for k, v := range info.Config.Labels {
		labels = append(labels, fmt.Sprintf("--label %s=%s", k, v))
	}

	runCmd := fmt.Sprintf(`docker run -d 
		--name %[1]s_new 
		--network %[2]s 
		--network-alias %[1]s_new 
		-e %[3]s 
		-v %[4]s 
		%[5]s 
		--health-cmd "curl -f http://localhost:8080/health || exit 1" 
		--health-interval=5s 
		--health-retries=3 
		--health-timeout=2s 
		%[6]s`,
		service, network, envVars, volumeBinds, strings.Join(labels, " "), imageName)

	_, err = d.client.RunCommand(runCmd)
	return err
}

func (d *DockerUpdater) performHealthChecks(container string) error {
	for i := 0; i < healthCheckRetries; i++ {
		cmd := fmt.Sprintf("docker inspect --format='{{.State.Health.Status}}' %s", container)
		output, err := d.client.RunCommand(cmd)
		if err == nil && strings.TrimSpace(string(output)) == "healthy" {
			return nil
		}
		time.Sleep(healthCheckInterval)
	}
	return fmt.Errorf("container failed to become healthy")
}

func (d *DockerUpdater) switchTraffic(service, network string) error {
	newContainer := service + "_new"
	oldContainer, err := d.getContainerID(service, network)
	if err != nil {
		return fmt.Errorf("failed to get old container ID: %v", err)
	}

	cmds := []string{
		fmt.Sprintf("docker network disconnect %s %s", network, newContainer),
		fmt.Sprintf("docker network connect --alias %s %s %s", service, network, newContainer),
		fmt.Sprintf("docker network disconnect %s %s", network, oldContainer),
	}

	for _, cmd := range cmds {
		if _, err := d.client.RunCommand(cmd); err != nil {
			return fmt.Errorf("failed to execute command '%s': %v", cmd, err)
		}
	}

	return nil
}

func (d *DockerUpdater) cleanup(service, network string) error {
	oldContainer, err := d.getContainerID(service, network)
	if err != nil {
		return fmt.Errorf("failed to get old container ID: %v", err)
	}

	cmds := []string{
		fmt.Sprintf("docker stop %s", oldContainer),
		fmt.Sprintf("docker rm %s", oldContainer),
		fmt.Sprintf("docker rename %s_new %s", service, service),
	}

	for _, cmd := range cmds {
		if _, err := d.client.RunCommand(cmd); err != nil {
			return fmt.Errorf("failed to execute command '%s': %v", cmd, err)
		}
	}

	return nil
}

func (d *DockerUpdater) pullImage(imageName string) error {
	cmd := fmt.Sprintf("docker pull %s", imageName)
	_, err := d.client.RunCommand(cmd)
	return err
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

	if err := d.performHealthChecks(service + "_new"); err != nil {
		if _, rmErr := d.client.RunCommand(fmt.Sprintf("docker rm -f %s_new", service)); rmErr != nil {
			return fmt.Errorf("update failed for %s: new container is unhealthy and cleanup failed: %v", service, rmErr)
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
