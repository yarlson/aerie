package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "aerie",
		Short: "Set up a new user, Docker, and UFW on a remote server",
		Run:   runSetup,
	}

	rootCmd.Flags().StringP("host", "H", "", "SSH host IP address")
	rootCmd.Flags().StringP("user", "u", "", "New user to create")
	rootCmd.Flags().StringP("ssh-key", "k", "", "Path to the SSH public key file for the new user")
	rootCmd.Flags().StringP("root-key", "r", "", "Path to the root SSH private key file")

	_ = rootCmd.MarkFlagRequired("host")
	_ = rootCmd.MarkFlagRequired("user")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runSetup(cmd *cobra.Command, args []string) {
	host, _ := cmd.Flags().GetString("host")
	newUser, _ := cmd.Flags().GetString("user")
	sshKeyPath, _ := cmd.Flags().GetString("ssh-key")
	rootKeyPath, _ := cmd.Flags().GetString("root-key")

	// Find appropriate SSH keys
	rootKey, err := findSSHKey(rootKeyPath, true)
	if err != nil {
		fmt.Printf("Failed to find root SSH key: %v\n", err)
		return
	}

	userKey, err := findSSHKey(sshKeyPath, false)
	if err != nil {
		fmt.Printf("Failed to find user SSH key, using root key for new user\n")
		userKey = rootKey
	}

	// Create signer
	signer, err := ssh.ParsePrivateKey(rootKey)
	if err != nil {
		fmt.Printf("Failed to parse private key: %v\n", err)
		return
	}

	// Prompt for new user password
	fmt.Print("Enter password for new user: ")
	newUserPassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Printf("\nFailed to read new user password: %v\n", err)
		return
	}
	fmt.Println()

	// Connect to SSH
	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer client.Close()

	// Create user
	if err := runCommand(client, fmt.Sprintf("adduser --gecos '' --disabled-password %s", newUser)); err != nil {
		fmt.Printf("Failed to create user: %v\n", err)
		return
	}

	if err := runCommand(client, fmt.Sprintf("echo '%s:%s' | chpasswd", newUser, string(newUserPassword))); err != nil {
		fmt.Printf("Failed to set user password: %v\n", err)
		return
	}

	// Set up SSH key for new user
	userPubKey, err := ssh.ParsePrivateKey(userKey)
	if err != nil {
		fmt.Printf("Failed to parse user private key: %v\n", err)
		return
	}
	userPubKeyString := string(ssh.MarshalAuthorizedKey(userPubKey.PublicKey()))

	if err := runCommand(client, fmt.Sprintf("mkdir -p /home/%s/.ssh", newUser)); err != nil {
		fmt.Printf("Failed to create .ssh directory: %v\n", err)
		return
	}

	if err := runCommand(client, fmt.Sprintf("echo '%s' >> /home/%s/.ssh/authorized_keys", userPubKeyString, newUser)); err != nil {
		fmt.Printf("Failed to add SSH key: %v\n", err)
		return
	}

	// Install Docker and Docker Compose
	commands := []string{
		"apt-get update",
		"apt-get install -y apt-transport-https ca-certificates curl software-properties-common",
		"curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -",
		"add-apt-repository \"deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable\"",
		"apt-get update",
		"apt-get install -y docker-ce docker-ce-cli containerd.io",
		"curl -L \"https://github.com/docker/compose/releases/download/1.29.2/docker-compose-$(uname -s)-$(uname -m)\" -o /usr/local/bin/docker-compose",
		"chmod +x /usr/local/bin/docker-compose",
	}

	for _, cmd := range commands {
		if err := runCommand(client, cmd); err != nil {
			fmt.Printf("Failed to run command '%s': %v\n", cmd, err)
			return
		}
	}

	// Install and configure UFW
	ufwCommands := []string{
		"apt-get install -y ufw",
		"ufw default deny incoming",
		"ufw default allow outgoing",
		"ufw allow 22/tcp",
		"ufw allow 80/tcp",
		"ufw allow 443/tcp",
		"echo 'y' | ufw enable",
	}

	for _, cmd := range ufwCommands {
		if err := runCommand(client, cmd); err != nil {
			fmt.Printf("Failed to run UFW command '%s': %v\n", cmd, err)
			return
		}
	}

	fmt.Println("Setup completed successfully!")
}

func findSSHKey(keyPath string, isRoot bool) ([]byte, error) {
	if keyPath != "" {
		return os.ReadFile(keyPath)
	}

	// Try to find key in .ssh directory
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %v", err)
	}

	sshDir := filepath.Join(home, ".ssh")
	keyNames := []string{"id_rsa", "id_ecdsa", "id_ed25519"}

	for _, name := range keyNames {
		path := filepath.Join(sshDir, name)
		if _, err := os.Stat(path); err == nil {
			key, err := os.ReadFile(path)
			if err == nil {
				return key, nil
			}
		}
	}

	if isRoot {
		return nil, fmt.Errorf("no suitable SSH key found in .ssh directory")
	}
	return nil, nil
}

func runCommand(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	return session.Run(command)
}
