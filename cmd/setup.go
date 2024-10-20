package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/yarlson/ftl/pkg/config"
	"github.com/yarlson/ftl/pkg/console"
	"github.com/yarlson/ftl/pkg/setup"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Prepare servers for deployment",
	Long: `Setup configures servers defined in ftl.yaml for deployment.
Run this once for each new server before deploying your application.`,
	Run: runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) {
	cfg, err := parseConfig("ftl.yaml")
	if err != nil {
		console.ErrPrintln("Failed to parse config file:", err)
		return
	}

	console.Input("Enter server user password:")
	newUserPassword, err := console.ReadPassword()
	if err != nil {
		console.ErrPrintln("Failed to read password:", err)
		return
	}
	console.Success("\nServer user password received.")

	for _, server := range cfg.Servers {
		if err := setupServer(server, string(newUserPassword)); err != nil {
			console.ErrPrintln(fmt.Sprintf("Failed to setup server %s:", server.Host), err)
			continue
		}
		console.Success(fmt.Sprintf("Successfully set up server %s", server.Host))
	}

	console.Success("Server setup completed successfully.")
}

func setupServer(server config.Server, newUserPassword string) error {
	console.Info(fmt.Sprintf("Setting up server %s...", server.Host))

	sshKeyPath := filepath.Join(os.Getenv("HOME"), ".ssh", filepath.Base(server.SSHKey))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return setup.RunSetup(ctx, server, sshKeyPath, newUserPassword)
}
