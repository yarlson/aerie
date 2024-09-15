package cmd

import (
	"github.com/enclave-ci/aerie/internal/deploy"
	"github.com/spf13/cobra"
)

var (
	deployHost       string
	deployUser       string
	deploySSHKeyPath string
	appDir           string
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy your application seamlessly",
	Run:   deploy.RunDeploy,
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
