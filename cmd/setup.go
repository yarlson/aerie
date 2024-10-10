package cmd

import (
	"github.com/spf13/cobra"

	"github.com/yarlson/aerie/pkg/setup"
)

var (
	host        string
	port        int
	defaultUser string
	newUser     string
	sshKeyPath  string
	rootKeyPath string
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup a new user, install essential software, and configure UFW on a remote server",
	Run:   setup.RunSetup,
}

func init() {
	setupCmd.Flags().StringVarP(&host, "host", "H", "", "SSH host IP address")
	setupCmd.Flags().IntVarP(&port, "port", "p", 22, "SSH port")
	setupCmd.Flags().StringVarP(&defaultUser, "default-user", "d", "", "Default user name")
	setupCmd.Flags().StringVarP(&newUser, "user", "u", "", "New user to create")
	setupCmd.Flags().StringVarP(&sshKeyPath, "ssh-key", "k", "", "Path to the SSH public key file for the new user")
	setupCmd.Flags().StringVarP(&rootKeyPath, "root-key", "r", "", "Path to the root SSH private key file")

	_ = setupCmd.MarkFlagRequired("host")
	_ = setupCmd.MarkFlagRequired("user")

	rootCmd.AddCommand(setupCmd)
}
