package deploy

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/enclave-ci/aerie/internal/ssh"
	"github.com/enclave-ci/aerie/pkg/utils"
)

var (
	info       = color.New(color.FgCyan).PrintlnFunc()
	success    = color.New(color.FgGreen).PrintlnFunc()
	warning    = color.New(color.FgYellow).PrintlnFunc()
	errPrintln = color.New(color.FgRed).PrintlnFunc()
)

func RunDeploy(cmd *cobra.Command, args []string) {
	// Read flags
	host, _ := cmd.Flags().GetString("host")
	user, _ := cmd.Flags().GetString("user")
	sshKeyPath, _ := cmd.Flags().GetString("ssh-key")
	appDir, _ := cmd.Flags().GetString("app-dir")

	info("Starting deployment process...")

	if host == "" || user == "" {
		errPrintln("Host and user are required for deployment.")
		return
	}

	// Determine application version
	version := time.Now().Format("20060102150405") // Use timestamp as version

	// Build the Docker image
	if err := buildDockerImage(version, appDir); err != nil {
		errPrintln("Failed to build Docker image:", err)
		return
	}
	success("Docker image built successfully.")

	// Connect to the server
	info("Connecting to the server...")
	client, _, err := utils.FindKeyAndConnectWithUser(host, user, sshKeyPath)
	if err != nil {
		errPrintln("Failed to connect to the server:", err)
		return
	}
	defer client.Close()

	success("SSH connection to the server established.")

	// Transfer the Docker image to the server
	if err := transferDockerImage(client, version); err != nil {
		errPrintln("Failed to transfer Docker image to the server:", err)
		return
	}
	success("Docker image transferred to the server.")

	// Run Docker Compose commands on the server to deploy the new version
	if err := deployOnServer(client, version); err != nil {
		errPrintln("Failed to deploy on the server:", err)
		return
	}
	success("Deployment completed successfully.")
}

func buildDockerImage(version, appDir string) error {
	info("Building Docker image...")
	imageTag := fmt.Sprintf("myapp-web:%s", version)
	cmd := exec.Command("docker", "build", "-t", imageTag, appDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func transferDockerImage(client *ssh.Client, version string) error {
	info("Saving Docker image to a tar file...")
	imageTag := fmt.Sprintf("myapp-web:%s", version)
	tarFile := fmt.Sprintf("myapp-web-%s.tar", version)
	cmd := exec.Command("docker", "save", "-o", tarFile, imageTag)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	defer os.Remove(tarFile)

	info("Transferring Docker image to the server...")
	if err := client.UploadFile(tarFile, tarFile); err != nil {
		return err
	}

	// Load the image on the server
	info("Loading Docker image on the server...")
	if err := client.RunCommand(fmt.Sprintf("docker load -i %s", tarFile)); err != nil {
		return err
	}

	// Remove the tar file from the server
	return client.RunCommand(fmt.Sprintf("rm %s", tarFile))
}

func deployOnServer(client *ssh.Client, version string) error {
	info("Starting zero-downtime deployment on the server...")

	// Navigate to the application directory
	appDir := "~/app" // Assuming the app is in ~/app on the server
	cdCmd := fmt.Sprintf("cd %s", appDir)

	// Set environment variable VERSION on the server
	setVersionCmd := fmt.Sprintf("export VERSION=%s", version)

	// Pull the new image (if using a registry)
	// For this example, we assume the image is already loaded on the server

	// Scale up the service
	scaleUpCmd := "docker compose up -d --no-deps --no-recreate --scale web=2"
	fullScaleUpCmd := fmt.Sprintf("%s && %s && %s", cdCmd, setVersionCmd, scaleUpCmd)
	if err := client.RunCommandWithProgress(fullScaleUpCmd); err != nil {
		return err
	}

	// Wait for new container to start and pass health checks
	info("Waiting for the new container to become healthy...")
	if err := waitForServiceHealthy(client, "web", 120*time.Second); err != nil {
		// Rollback
		warning("Service did not become healthy, rolling back...")
		scaleDownCmd := "docker compose up -d --no-deps --no-recreate --scale web=1"
		fullScaleDownCmd := fmt.Sprintf("%s && %s", cdCmd, scaleDownCmd)
		if rollbackErr := client.RunCommandWithProgress(fullScaleDownCmd); rollbackErr != nil {
			return fmt.Errorf("failed to rollback: %v", rollbackErr)
		}
		return fmt.Errorf("deployment failed: %v", err)
	}

	// Scale down the old service
	scaleDownCmd := "docker compose up -d --no-deps --no-recreate --scale web=1"
	fullScaleDownCmd := fmt.Sprintf("%s && %s", cdCmd, scaleDownCmd)
	if err := client.RunCommandWithProgress(fullScaleDownCmd); err != nil {
		return err
	}

	return nil
}

func waitForServiceHealthy(client *ssh.Client, serviceName string, timeout time.Duration) error {
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("service %s did not become healthy within %v", serviceName, timeout)
		}

		cmd := fmt.Sprintf("docker inspect --format='{{.State.Health.Status}}' $(docker ps --filter 'name=%s' --format '{{.ID}}')", serviceName)
		output, err := client.RunCommandOutput(cmd)
		if err != nil {
			return fmt.Errorf("failed to check service health: %v", err)
		}
		if output == "healthy\n" || output == "healthy" {
			// Service is healthy
			return nil
		}
		// Sleep before retrying
		time.Sleep(5 * time.Second)
	}
}
