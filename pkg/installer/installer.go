package installer

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

type Executor interface {
	RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error)
}

type Installer struct {
	executor Executor
}

func NewInstaller(executor Executor) *Installer {
	return &Installer{executor: executor}
}

func (i *Installer) InstallService(service, network, imageName string) error {
	if err := i.pullImage(imageName); err != nil {
		return fmt.Errorf("failed to pull image for %s: %v", service, err)
	}

	if err := i.startContainer(service, network, imageName); err != nil {
		return fmt.Errorf("failed to start container for %s: %v", service, err)
	}

	return nil
}

type containerConfig struct {
	Env         []string
	Labels      map[string]string
	HealthCheck struct {
		Test     []string
		Timeout  time.Duration
		Retries  int
		Interval time.Duration
	}
	HostConfig struct {
		Binds []string
	}
}

func (i *Installer) startContainer(service, network, imageName string) error {
	config := i.getDefaultConfig()

	args := []string{"run", "-d", "--name", service, "--network", network, "--network-alias", service}

	for _, env := range config.Env {
		args = append(args, "-e", env)
	}

	for _, bind := range config.HostConfig.Binds {
		args = append(args, "-v", bind)
	}

	for k, v := range config.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}

	if len(config.HealthCheck.Test) > 0 {
		for _, test := range config.HealthCheck.Test {
			args = append(args, "--health-cmd", test)
		}
		args = append(args, "--health-interval", fmt.Sprintf("%ds", int(config.HealthCheck.Interval.Seconds())))
		args = append(args, "--health-retries", fmt.Sprintf("%d", config.HealthCheck.Retries))
		args = append(args, "--health-timeout", fmt.Sprintf("%ds", int(config.HealthCheck.Timeout.Seconds())))
	}

	args = append(args, imageName)

	_, err := i.runCommand(context.Background(), "docker", args...)
	return err
}

func (i *Installer) getDefaultConfig() containerConfig {
	return containerConfig{
		Env:    []string{"ENV=production"},
		Labels: map[string]string{"app": "myapp"},
		HealthCheck: struct {
			Test     []string
			Timeout  time.Duration
			Retries  int
			Interval time.Duration
		}{
			Test:     []string{"CMD", "curl -f http://localhost/ || exit 1"},
			Timeout:  time.Second * 30,
			Retries:  3,
			Interval: time.Second * 10,
		},
		HostConfig: struct{ Binds []string }{
			Binds: []string{"/host/path:/container/path"},
		},
	}
}

func (i *Installer) pullImage(imageName string) error {
	_, err := i.runCommand(context.Background(), "docker", "pull", imageName)
	return err
}

func (i *Installer) runCommand(ctx context.Context, command string, args ...string) (string, error) {
	output, err := i.executor.RunCommand(ctx, command, args...)
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
