package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "aerie",
	Short: "Aerie - Simplified server setup and application deployment",
	Long: `Aerie simplifies server setup and application deployment.
It provides zero-downtime deployments with automatic HTTPS.

Use 'aerie [command] --help' for more information about a command.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}
