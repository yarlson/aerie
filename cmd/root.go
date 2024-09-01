package cmd

import (
	"github.com/enclave-ci/aerie/internal/setup"
	"github.com/spf13/cobra"
)

var (
	host        string
	newUser     string
	sshKeyPath  string
	rootKeyPath string
)

var rootCmd = &cobra.Command{
	Use:   "aerie",
	Short: "Aerie sets up a new user, installs essential software, and configures UFW on a remote server",
	Run:   setup.RunSetup,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.Flags().StringVarP(&host, "host", "H", "", "SSH host IP address")
	rootCmd.Flags().StringVarP(&newUser, "user", "u", "", "New user to create")
	rootCmd.Flags().StringVarP(&sshKeyPath, "ssh-key", "k", "", "Path to the SSH public key file for the new user")
	rootCmd.Flags().StringVarP(&rootKeyPath, "root-key", "r", "", "Path to the root SSH private key file")

	_ = rootCmd.MarkFlagRequired("host")
	_ = rootCmd.MarkFlagRequired("user")
}
