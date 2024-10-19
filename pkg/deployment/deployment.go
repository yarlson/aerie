package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/yarlson/aerie/pkg/config"
	"github.com/yarlson/aerie/pkg/logfmt"
	"github.com/yarlson/aerie/pkg/proxy"
)

const (
	newContainerSuffix = "_new"
)

type Executor interface {
	RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error)
	CopyFile(ctx context.Context, from, to string) error
}

type Deployment struct {
	executor Executor
}

func NewDeployment(executor Executor) *Deployment {
	return &Deployment{executor: executor}
}

func (d *Deployment) Deploy(cfg *config.Config, network string) error {
	if err := logfmt.ProgressSpinner(context.Background(), "Creating network", "Network created", []func() error{
		func() error { return d.createNetwork(network) },
	}); err != nil {
		return fmt.Errorf("failed to create network: %w", err)
	}

	if err := logfmt.ProgressSpinner(context.Background(), "Creating volumes", "Volumes created", []func() error{
		func() error {
			for _, volume := range cfg.Volumes {
				if err := d.createVolume(volume); err != nil {
					return fmt.Errorf("failed to create volume: %w", err)
				}
			}
			return nil
		},
	}); err != nil {
		return fmt.Errorf("failed to create volumes: %w", err)
	}

	for _, service := range cfg.Services {
		if err := logfmt.ProgressSpinner(context.Background(),
			fmt.Sprintf("Deploying service: %s", service.Name),
			fmt.Sprintf("Service deployed: %s", service.Name),
			[]func() error{
				func() error { return d.deployService(&service, network) },
			}); err != nil {
			return fmt.Errorf("failed to deploy service %s: %w", service.Name, err)
		}
	}

	if err := logfmt.ProgressSpinner(context.Background(), "Starting proxy", "Proxy started", []func() error{
		func() error { return d.StartProxy(cfg.Project.Name, cfg, network) },
	}); err != nil {
		return fmt.Errorf("failed to start proxy: %w", err)
	}

	return nil
}

func (d *Deployment) StartProxy(project string, cfg *config.Config, network string) error {
	projectPath, err := d.prepareProjectFolder(project)
	if err != nil {
		return fmt.Errorf("failed to prepare project folder: %w", err)
	}

	configPath, err := d.prepareNginxConfig(cfg, projectPath)
	if err != nil {
		return fmt.Errorf("failed to prepare nginx config: %w", err)
	}

	service := &config.Service{
		Name:  "proxy",
		Image: "yarlson/zero-nginx:latest",
		Port:  80,
		Volumes: []string{
			projectPath + "/:/etc/nginx/ssl",
			configPath + ":/etc/nginx/conf.d",
		},
		EnvVars: []config.EnvVar{
			{
				Name:  "DOMAIN",
				Value: cfg.Project.Domain,
			},
			{
				Name:  "EMAIL",
				Value: cfg.Project.Email,
			},
		},
		Forwards: []string{
			"80:80",
			"443:443",
		},
		HealthCheck: &config.HealthCheck{
			Path:     "/",
			Interval: time.Second,
			Timeout:  time.Second,
			Retries:  30,
		},
	}

	if err := d.deployService(service, network); err != nil {
		return fmt.Errorf("failed to deploy service %s: %w", service.Name, err)
	}

	return nil
}

func (d *Deployment) InstallService(service *config.Service, network string) error {
	if _, err := d.pullImage(service.Image); err != nil {
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

	if _, err := d.pullImage(service.Image); err != nil {
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

	oldContID, err := d.switchTraffic(svcName, network)
	if err != nil {
		return fmt.Errorf("failed to switch traffic for %s: %v", svcName, err)
	}

	if err := d.cleanup(oldContID, svcName); err != nil {
		return fmt.Errorf("failed to cleanup for %s: %v", svcName, err)
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
	Image           string
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
	output, err := d.runCommand(context.Background(), "docker", "ps", "-aq", "--filter", fmt.Sprintf("network=%s", network))
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
		args = append(args, "--health-cmd", fmt.Sprintf("curl -f http://localhost:%d%s || exit 1", service.Port, service.HealthCheck.Path))
		args = append(args, "--health-interval", fmt.Sprintf("%ds", int(service.HealthCheck.Interval.Seconds())))
		args = append(args, "--health-retries", fmt.Sprintf("%d", service.HealthCheck.Retries))
		args = append(args, "--health-timeout", fmt.Sprintf("%ds", int(service.HealthCheck.Timeout.Seconds())))
	}

	if len(service.Forwards) > 0 {
		for _, forward := range service.Forwards {
			args = append(args, "-p", forward)
		}
	}

	hash, err := ServiceHash(service)
	if err != nil {
		return fmt.Errorf("failed to generate config hash: %w", err)
	}
	args = append(args, "--label", fmt.Sprintf("aerie.config-hash=%s", hash))
	args = append(args, service.Image)

	_, err = d.runCommand(context.Background(), "docker", args...)
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

func (d *Deployment) switchTraffic(service, network string) (string, error) {
	newContainer := service + newContainerSuffix
	oldContainer, err := d.getContainerID(service, network)
	if err != nil {
		return "", fmt.Errorf("failed to get old container ID: %v", err)
	}

	cmds := [][]string{
		{"docker", "network", "disconnect", network, newContainer},
		{"docker", "network", "connect", "--alias", service, network, newContainer},
	}

	for _, cmd := range cmds {
		if _, err := d.runCommand(context.Background(), cmd[0], cmd[1:]...); err != nil {
			return "", fmt.Errorf("failed to execute command '%s': %v", strings.Join(cmd, " "), err)
		}
	}

	time.Sleep(1 * time.Second)

	cmds = [][]string{
		{"docker", "network", "disconnect", network, oldContainer},
	}

	for _, cmd := range cmds {
		if _, err := d.runCommand(context.Background(), cmd[0], cmd[1:]...); err != nil {
			return "", fmt.Errorf("failed to execute command '%s': %v", strings.Join(cmd, " "), err)
		}
	}

	return oldContainer, nil
}

func (d *Deployment) cleanup(oldContID, service string) error {
	cmds := [][]string{
		{"docker", "stop", oldContID},
		{"docker", "rm", oldContID},
		{"docker", "rename", service + newContainerSuffix, service},
	}

	for _, cmd := range cmds {
		if _, err := d.runCommand(context.Background(), cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("failed to execute command '%s': %v", strings.Join(cmd, " "), err)
		}
	}

	return nil
}

func (d *Deployment) pullImage(imageName string) (string, error) {
	_, err := d.runCommand(context.Background(), "docker", "pull", imageName)
	if err != nil {
		return "", err
	}

	output, err := d.runCommand(context.Background(), "docker", "images", "--no-trunc", "--format={{.ID}}", imageName)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(output), nil
}

func (d *Deployment) runCommand(ctx context.Context, command string, args ...string) (string, error) {
	output, err := d.executor.RunCommand(ctx, command, args...)
	outputBytes, readErr := io.ReadAll(output)
	if readErr != nil {
		return "", fmt.Errorf("failed to read command output: %v (original error: %w)", readErr, err)
	}

	result := strings.TrimSpace(string(outputBytes))

	if err != nil {
		return result, fmt.Errorf("command failed: %w", err)
	}

	return result, nil
}

func (d *Deployment) makeProjectFolder(projectName string) error {
	projectPath, err := d.projectFolder(projectName)
	if err != nil {
		return fmt.Errorf("failed to get project folder path: %w", err)
	}

	_, err = d.runCommand(context.Background(), "mkdir", "-p", projectPath)
	return err
}

func (d *Deployment) projectFolder(projectName string) (string, error) {
	homeDir, err := d.runCommand(context.Background(), "sh", "-c", "echo $HOME")
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	homeDir = strings.TrimSpace(homeDir)
	projectPath := filepath.Join(homeDir, "projects", projectName)

	return projectPath, nil
}

func (d *Deployment) prepareProjectFolder(project string) (string, error) {
	if err := d.makeProjectFolder(project); err != nil {
		return "", fmt.Errorf("failed to create project folder: %w", err)
	}

	return d.projectFolder(project)
}

func (d *Deployment) prepareNginxConfig(cfg *config.Config, projectPath string) (string, error) {
	nginxConfig, err := proxy.GenerateNginxConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to generate nginx config: %w", err)
	}

	nginxConfig = strings.TrimSpace(nginxConfig)

	configPath := filepath.Join(projectPath, "nginx")
	_, err = d.runCommand(context.Background(), "mkdir", "-p", configPath)
	if err != nil {
		return "", fmt.Errorf("failed to create nginx config directory: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "nginx-config-*.conf")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(nginxConfig); err != nil {
		return "", fmt.Errorf("failed to write nginx config to temporary file: %w", err)
	}

	return configPath, d.executor.CopyFile(context.Background(), tmpFile.Name(), filepath.Join(configPath, "default.conf"))
}

func ServiceHash(service *config.Service) (string, error) {
	sortedService := sortServiceFields(service)
	bytes, err := json.Marshal(sortedService)
	if err != nil {
		return "", fmt.Errorf("failed to marshal sorted service: %w", err)
	}

	hash := sha256.Sum256(bytes)
	return hex.EncodeToString(hash[:]), nil
}

func sortServiceFields(service *config.Service) map[string]interface{} {
	sorted := make(map[string]interface{})
	v := reflect.ValueOf(*service)
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i).Interface()

		switch reflect.TypeOf(value).Kind() {
		case reflect.Slice:
			s := reflect.ValueOf(value)
			sorted[field.Name] = sortSlice(s)
		case reflect.Map:
			m := reflect.ValueOf(value)
			sorted[field.Name] = sortMap(m)
		default:
			sorted[field.Name] = value
		}
	}

	return sorted
}

func sortSlice(s reflect.Value) []interface{} {
	sorted := make([]interface{}, s.Len())
	for i := 0; i < s.Len(); i++ {
		sorted[i] = s.Index(i).Interface()
	}
	sort.Slice(sorted, func(i, j int) bool {
		return fmt.Sprintf("%v", sorted[i]) < fmt.Sprintf("%v", sorted[j])
	})
	return sorted
}

func sortMap(m reflect.Value) map[string]interface{} {
	sorted := make(map[string]interface{})
	for _, key := range m.MapKeys() {
		sorted[key.String()] = m.MapIndex(key).Interface()
	}
	return sorted
}

func (d *Deployment) serviceChanged(service *config.Service, network string) (bool, error) {
	containerInfo, err := d.getContainerInfo(service.Name, network)
	if err != nil {
		return false, fmt.Errorf("failed to get container info: %w", err)
	}

	hash, err := ServiceHash(service)
	if err != nil {
		return false, fmt.Errorf("failed to generate config hash: %w", err)
	}

	return containerInfo.Config.Labels["aerie.config-hash"] != hash, nil
}

func (d *Deployment) deployService(service *config.Service, network string) error {
	hash, err := d.pullImage(service.Image)
	if err != nil {
		return fmt.Errorf("failed to pull image for %s: %w", service.Name, err)
	}

	containerInfo, err := d.getContainerInfo(service.Name, network)
	if err != nil {
		if err := d.InstallService(service, network); err != nil {
			return fmt.Errorf("failed to install service %s: %w", service.Name, err)
		}

		return nil
	}

	if hash != containerInfo.Image {
		if err := d.UpdateService(service, network); err != nil {
			return fmt.Errorf("failed to update service %s due to image change: %w", service.Name, err)
		}

		return nil
	}

	changed, err := d.serviceChanged(service, network)
	if err != nil {
		return fmt.Errorf("failed to check if service %s has changed: %w", service.Name, err)
	}

	if changed {
		if err := d.UpdateService(service, network); err != nil {
			return fmt.Errorf("failed to update service %s due to config change: %w", service.Name, err)
		}
	}

	return nil
}

func (d *Deployment) networkExists(network string) (bool, error) {
	output, err := d.runCommand(context.Background(), "docker", "network", "ls", "--format", "{{.Name}}")
	if err != nil {
		return false, fmt.Errorf("failed to list Docker networks: %w", err)
	}

	networks := strings.Split(strings.TrimSpace(output), "\n")
	for _, n := range networks {
		if strings.TrimSpace(n) == network {
			return true, nil
		}
	}
	return false, nil
}

func (d *Deployment) createNetwork(network string) error {
	exists, err := d.networkExists(network)
	if err != nil {
		return fmt.Errorf("failed to check if network exists: %w", err)
	}

	if exists {
		return nil
	}

	_, err = d.runCommand(context.Background(), "docker", "network", "create", network)
	if err != nil {
		return fmt.Errorf("failed to create network: %w", err)
	}

	return nil
}

func (d *Deployment) createVolume(volume string) error {
	if _, err := d.runCommand(context.Background(), "docker", "volume", "inspect", volume); err == nil {
		return nil
	}

	_, err := d.runCommand(context.Background(), "docker", "volume", "create", volume)
	if err != nil {
		return fmt.Errorf("failed to create volume: %w", err)
	}

	return nil
}
