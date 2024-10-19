package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yarlson/ftl/pkg/config"
	"github.com/yarlson/ftl/pkg/console"
	"github.com/yarlson/ftl/pkg/deployment"
	"github.com/yarlson/ftl/pkg/ssh"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy your application to configured servers",
	Long: `Deploy your application to all servers defined in ftl.yaml.
This command handles the entire deployment process, ensuring
zero-downtime updates of your services.`,
	Run: runDeploy,
}

func init() {
	rootCmd.AddCommand(deployCmd)
}

func runDeploy(cmd *cobra.Command, args []string) {
	cfg, err := parseConfig("ftl.yaml")
	if err != nil {
		console.ErrPrintln("Failed to parse config file:", err)
		return
	}

	networkName := fmt.Sprintf("%s-network", cfg.Project.Name)

	for _, server := range cfg.Servers {
		if err := deployToServer(cfg, server, networkName); err != nil {
			console.ErrPrintln(fmt.Sprintf("Failed to deploy to server %s:", server.Host), err)
			continue
		}
		console.Success(fmt.Sprintf("Successfully deployed to server %s", server.Host))
	}

	console.Success("Deployment completed successfully.")
}

func parseConfig(filename string) (*config.Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg, err := config.ParseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

func deployToServer(cfg *config.Config, server config.Server, networkName string) error {
	console.Info(fmt.Sprintf("Deploying to server %s...", server.Host))

	sshKeyPath := filepath.Join(os.Getenv("HOME"), ".ssh", filepath.Base(server.SSHKey))
	client, _, err := ssh.FindKeyAndConnectWithUser(server.Host, server.Port, server.User, sshKeyPath)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	deploy := deployment.NewDeployment(client)

	if err := deploy.Deploy(cfg, networkName); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	return nil
}
