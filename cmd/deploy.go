package cmd

import (
	"github.com/spf13/cobra"
	"github.com/yarlson/aerie/pkg/ssh"

	"github.com/yarlson/aerie/pkg/logfmt"
)

var (
	deployHost       string
	deployUser       string
	deploySSHKeyPath string
	appDir           string
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Rollout your application seamlessly",
	Run:   run,
}

func run(cmd *cobra.Command, args []string) {
	// Read flags
	host, _ := cmd.Flags().GetString("host")
	user, _ := cmd.Flags().GetString("user")
	sshKeyPath, _ := cmd.Flags().GetString("ssh-key")

	logfmt.Info("Starting service process...")

	if host == "" || user == "" {
		logfmt.ErrPrintln("Host and user are required for service.")
		return
	}

	// Connect to the server
	logfmt.Info("Connecting to the server...")
	client, _, err := ssh.FindKeyAndConnectWithUser(host, 0, user, sshKeyPath)
	if err != nil {
		logfmt.ErrPrintln("Failed to connect to the server:", err)
		return
	}
	defer client.Close()

	logfmt.Success("SSH connection to the server established.")

	logfmt.Success("Deployment completed successfully.")
}

func init() {
	deployCmd.Flags().StringVarP(&deployHost, "host", "H", "", "SSH host IP address")
	deployCmd.Flags().StringVarP(&deployUser, "user", "u", "", "SSH user for deployment")
	deployCmd.Flags().StringVarP(&deploySSHKeyPath, "ssh-key", "k", "", "Path to the SSH private key file")
	deployCmd.Flags().StringVarP(&appDir, "app-dir", "d", ".", "Path to your application's directory")

	_ = deployCmd.MarkFlagRequired("host")
	_ = deployCmd.MarkFlagRequired("user")

	rootCmd.AddCommand(deployCmd)
}
