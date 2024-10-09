package pkg

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yarlson/aerie/pkg/ssh"
)

const (
	dockerNetwork       = "pdg_default"
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
		Image string
	}
	NetworkSettings struct {
		Networks map[string]struct {
			Aliases []string
		}
	}
}

func (d *DockerUpdater) getImageName(service string) (string, error) {
	cmd := fmt.Sprintf("docker ps -q --filter network=%s", dockerNetwork)
	output, err := d.client.RunCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get container IDs: %v", err)
	}

	containerIDs := strings.Fields(string(output))
	for _, cid := range containerIDs {
		cmd = fmt.Sprintf("docker inspect %s", cid)
		inspectOutput, err := d.client.RunCommand(cmd)
		if err != nil {
			continue
		}

		var containerInfo []containerInfo

		if err := json.Unmarshal(inspectOutput, &containerInfo); err != nil {
			continue
		}

		if len(containerInfo) == 0 {
			continue
		}

		if aliases, ok := containerInfo[0].NetworkSettings.Networks[dockerNetwork]; ok {
			for _, alias := range aliases.Aliases {
				if alias == service {
					return containerInfo[0].Config.Image, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no container found with alias %s in network %s", service, dockerNetwork)
}

func (d *DockerUpdater) getContainerID(service string) (string, error) {
	cmd := fmt.Sprintf("docker ps -q --filter network=%s", dockerNetwork)
	output, err := d.client.RunCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get container IDs: %v", err)
	}

	containerIDs := strings.Fields(string(output))
	for _, cid := range containerIDs {
		cmd = fmt.Sprintf("docker inspect %s", cid)
		inspectOutput, err := d.client.RunCommand(cmd)
		if err != nil {
			continue
		}

		var containerInfo []containerInfo

		if err := json.Unmarshal([]byte(inspectOutput), &containerInfo); err != nil {
			continue
		}

		if len(containerInfo) == 0 {
			continue
		}

		if aliases, ok := containerInfo[0].NetworkSettings.Networks[dockerNetwork]; ok {
			for _, alias := range aliases.Aliases {
				if alias == service {
					return containerInfo[0].ID, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no container found with alias %s in network %s", service, dockerNetwork)
}

func (d *DockerUpdater) startNewContainer(service string) error {
	imageName, err := d.getImageName(service)
	if err != nil {
		return fmt.Errorf("failed to get image name: %v", err)
	}

	currentContainer, err := d.getContainerID(service)
	if err != nil {
		return fmt.Errorf("failed to get current container ID: %v", err)
	}

	cmd := fmt.Sprintf("docker inspect %s", currentContainer)
	inspectOutput, err := d.client.RunCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to inspect current container: %v", err)
	}

	var containerInfo []struct {
		Config struct {
			Env    []string
			Labels map[string]string
		}
		HostConfig struct {
			Binds []string
		}
	}

	if err := json.Unmarshal(inspectOutput, &containerInfo); err != nil {
		return fmt.Errorf("failed to parse inspect output: %v", err)
	}

	if len(containerInfo) == 0 {
		return fmt.Errorf("no inspect info found for container %s", currentContainer)
	}

	envVars := strings.Join(containerInfo[0].Config.Env, " -e ")
	volumeBinds := strings.Join(containerInfo[0].HostConfig.Binds, " -v ")
	labels := ""
	for k, v := range containerInfo[0].Config.Labels {
		labels += fmt.Sprintf(" --label %s=%s", k, v)
	}

	runCmd := fmt.Sprintf(`docker run -d 
		--name %s_new 
		--network %s 
		--network-alias %s_new 
		-e %s 
		-v %s 
		%s 
		--health-cmd "curl -f http://localhost:8080/health || exit 1" 
		--health-interval=5s 
		--health-retries=3 
		--health-timeout=2s 
		%s`,
		service, dockerNetwork, service, envVars, volumeBinds, labels, imageName)

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

func (d *DockerUpdater) switchTraffic(service string) error {
	newContainer := service + "_new"
	oldContainer, err := d.getContainerID(service)
	if err != nil {
		return fmt.Errorf("failed to get old container ID: %v", err)
	}

	cmds := []string{
		fmt.Sprintf("docker network disconnect %s %s", dockerNetwork, newContainer),
		fmt.Sprintf("docker network connect --alias %s %s %s", service, dockerNetwork, newContainer),
		fmt.Sprintf("docker network disconnect %s %s", dockerNetwork, oldContainer),
	}

	for _, cmd := range cmds {
		if _, err := d.client.RunCommand(cmd); err != nil {
			return fmt.Errorf("failed to execute command '%s': %v", cmd, err)
		}
	}

	return nil
}

func (d *DockerUpdater) cleanup(service string) error {
	oldContainer, err := d.getContainerID(service)
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

func (d *DockerUpdater) UpdateService(service string) error {
	imageName, err := d.getImageName(service)
	if err != nil {
		return fmt.Errorf("failed to get image name for %s: %v", service, err)
	}

	if err := d.pullImage(imageName); err != nil {
		return fmt.Errorf("failed to pull new image for %s: %v", service, err)
	}

	if err := d.startNewContainer(service); err != nil {
		return fmt.Errorf("failed to start new container for %s: %v", service, err)
	}

	if err := d.performHealthChecks(service + "_new"); err != nil {
		if _, rmErr := d.client.RunCommand(fmt.Sprintf("docker rm -f %s_new", service)); rmErr != nil {
			return fmt.Errorf("update failed for %s: new container is unhealthy and cleanup failed: %v", service, rmErr)
		}
		return fmt.Errorf("update failed for %s: new container is unhealthy: %w", service, err)
	}

	if err := d.switchTraffic(service); err != nil {
		return fmt.Errorf("failed to switch traffic for %s: %v", service, err)
	}

	if err := d.cleanup(service); err != nil {
		return fmt.Errorf("failed to cleanup for %s: %v", service, err)
	}

	return nil
}
