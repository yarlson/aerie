package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/yarlson/aerie/pkg/config"
	"github.com/yarlson/aerie/pkg/logfmt"
	"github.com/yarlson/aerie/pkg/setup"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Prepare servers for deployment",
	Long: `Setup configures servers defined in aerie.yaml for deployment.
Run this once for each new server before deploying your application.`,
	Run: runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) {
	cfg, err := parseConfig("aerie.yaml")
	if err != nil {
		logfmt.ErrPrintln("Failed to parse config file:", err)
		return
	}

	for _, server := range cfg.Servers {
		if err := setupServer(server); err != nil {
			logfmt.ErrPrintln(fmt.Sprintf("Failed to setup server %s:", server.Host), err)
			continue
		}
		logfmt.Success(fmt.Sprintf("Successfully set up server %s", server.Host))
	}

	logfmt.Success("Server setup completed successfully.")
}

func setupServer(server config.Server) error {
	logfmt.Info(fmt.Sprintf("Setting up server %s...", server.Host))

	sshKeyPath := filepath.Join(os.Getenv("HOME"), ".ssh", filepath.Base(server.SSHKey))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return setup.RunSetup(ctx, server, sshKeyPath)
}
