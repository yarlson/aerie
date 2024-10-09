package deployment

import (
	"fmt"
	"github.com/yarlson/aerie/pkg/logfmt"
	"os"
	"os/exec"
	"time"

	"github.com/yarlson/aerie/pkg/ssh"
)

type Service struct {
	client *ssh.Client
}

func NewService(client *ssh.Client) *Service {
	return &Service{
		client: client,
	}
}

func (s *Service) Rollout(appDir string) error {
	// Determine application version
	version := time.Now().Format("20060102150405") // Use timestamp as version

	// Build the Docker image
	if err := s.BuildDockerImage(version, appDir); err != nil {
		return fmt.Errorf("failed to build Docker image: %v", err)
	}
	logfmt.Success("Docker image built successfully.")

	// Transfer the Docker image to the server
	if err := s.TransferDockerImage(version); err != nil {
		return fmt.Errorf("failed to transfer Docker image to the server: %v", err)
	}
	logfmt.Success("Docker image transferred to the server.")

	// Run Docker Compose commands on the server to deploy the new version
	if err := s.DeployOnServer(version); err != nil {
		return fmt.Errorf("failed to deploy on the server: %v", err)
	}

	return nil
}

func (s *Service) BuildDockerImage(version, appDir string) error {
	logfmt.Info("Building Docker image...")
	imageTag := fmt.Sprintf("myapp-web:%s", version)
	cmd := exec.Command("docker", "build", "-t", imageTag, appDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *Service) TransferDockerImage(version string) error {
	// Save the Docker image to a tar file
	logfmt.Info("Saving Docker image to a tar file...")
	imageTag := fmt.Sprintf("myapp-web:%s", version)
	tarFile := fmt.Sprintf("myapp-web-%s.tar", version)
	cmd := exec.Command("docker", "save", "-o", tarFile, imageTag)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	defer os.Remove(tarFile)

	// Transfer the tar file to the server
	logfmt.Info("Transferring Docker image to the server...")
	if err := s.client.UploadFile(tarFile, tarFile); err != nil {
		return err
	}

	// Load the image on the server
	commands := []string{
		fmt.Sprintf("docker load -i %s", tarFile),
		fmt.Sprintf("rm %s", tarFile), // Remove the tar file from the server
	}

	if err := s.client.RunCommandWithProgress(
		"Loading Docker image on the server...",
		"Docker image loaded on the server.",
		commands,
	); err != nil {
		return err
	}

	return nil
}

func (s *Service) DeployOnServer(version string) error {
	logfmt.Info("Starting zero-downtime deployment on the server...")

	// Commands to deploy the new version
	appDir := "~/app" // Assuming the app is in ~/app on the server
	commands := []string{
		fmt.Sprintf("cd %s", appDir),
		fmt.Sprintf("export VERSION=%s", version),
		"docker compose up -d --no-deps --no-recreate --scale web=2",
	}

	if err := s.client.RunCommandWithProgress(
		"Deploying new version to the server...",
		"New version deployed successfully.",
		commands,
	); err != nil {
		return err
	}

	// Wait for the new container to become healthy
	if err := s.waitForServiceHealthy("web", 120*time.Second); err != nil {
		// Rollback
		logfmt.Warning("Service did not become healthy, rolling back...")
		rollbackCommands := []string{
			fmt.Sprintf("cd %s", appDir),
			"docker compose up -d --no-deps --no-recreate --scale web=1",
		}
		if rollbackErr := s.client.RunCommandWithProgress(
			"Rolling back deployment...",
			"Rollback completed.",
			rollbackCommands,
		); rollbackErr != nil {
			return fmt.Errorf("failed to rollback: %v", rollbackErr)
		}
		return fmt.Errorf("deployment failed: %v", err)
	}

	// Scale down the old service
	scaleDownCommands := []string{
		fmt.Sprintf("cd %s", appDir),
		"docker compose up -d --no-deps --no-recreate --scale web=1",
	}
	if err := s.client.RunCommandWithProgress(
		"Scaling down old version...",
		"Old version scaled down.",
		scaleDownCommands,
	); err != nil {
		return err
	}

	return nil
}

func (s *Service) waitForServiceHealthy(serviceName string, timeout time.Duration) error {
	logfmt.Info("Waiting for the new container to become healthy...")
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("service %s did not become healthy within %v", serviceName, timeout)
		}

		cmd := fmt.Sprintf("docker inspect --format='{{.State.Health.Status}}' $(docker ps --filter 'name=%s' --format '{{.ID}}')", serviceName)
		output, err := s.client.RunCommandOutput(cmd)
		if err != nil {
			return fmt.Errorf("failed to check service health: %v", err)
		}
		if output == "healthy\n" || output == "healthy" {
			// Service is healthy
			logfmt.Success("New container is healthy.")
			return nil
		}
		// Sleep before retrying
		time.Sleep(5 * time.Second)
	}
}
