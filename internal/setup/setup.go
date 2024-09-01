package setup

import (
	"fmt"
	"syscall"

	"github.com/enclave-ci/aerie/internal/ssh"
	"github.com/enclave-ci/aerie/pkg/utils"
	"github.com/spf13/cobra"
	gssh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func RunSetup(cmd *cobra.Command, args []string) {
	host, _ := cmd.Flags().GetString("host")
	newUser, _ := cmd.Flags().GetString("user")
	sshKeyPath, _ := cmd.Flags().GetString("ssh-key")
	rootKeyPath, _ := cmd.Flags().GetString("root-key")

	fmt.Println("Starting server setup process...")

	// Find appropriate SSH keys
	fmt.Println("Locating SSH keys...")
	rootKey, err := utils.FindSSHKey(rootKeyPath, true)
	if err != nil {
		fmt.Printf("Failed to find root SSH key: %v\n", err)
		return
	}
	fmt.Println("Root SSH key found.")

	userKey, err := utils.FindSSHKey(sshKeyPath, false)
	if err != nil {
		fmt.Println("Failed to find user SSH key, using root key for new user")
		userKey = rootKey
	} else {
		fmt.Println("User SSH key found.")
	}

	// Prompt for new user password
	fmt.Print("Enter password for new user: ")
	newUserPassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Printf("\nFailed to read new user password: %v\n", err)
		return
	}
	fmt.Println("\nPassword received.")

	// Connect to SSH
	client, err := ssh.Connect(host, rootKey)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer client.Close()

	// Perform setup steps
	if err := createUser(client, newUser, string(newUserPassword)); err != nil {
		return
	}

	if err := setupSSHKey(client, newUser, userKey); err != nil {
		return
	}

	if err := installSoftware(client); err != nil {
		return
	}

	if err := configureUFW(client); err != nil {
		return
	}

	fmt.Println("Setup completed successfully!")
}

func createUser(client *ssh.Client, newUser, password string) error {
	fmt.Printf("Creating new user: %s\n", newUser)
	if err := client.RunCommand(fmt.Sprintf("adduser --gecos '' --disabled-password %s", newUser)); err != nil {
		fmt.Printf("Failed to create user: %v\n", err)
		return err
	}

	if err := client.RunCommand(fmt.Sprintf("echo '%s:%s' | chpasswd", newUser, password)); err != nil {
		fmt.Printf("Failed to set user password: %v\n", err)
		return err
	}
	fmt.Println("User created successfully.")
	return nil
}

func setupSSHKey(client *ssh.Client, newUser string, userKey []byte) error {
	fmt.Println("Setting up SSH key for new user...")
	userPubKey, err := gssh.ParsePrivateKey(userKey)
	if err != nil {
		fmt.Printf("Failed to parse user private key: %v\n", err)
		return err
	}
	userPubKeyString := string(gssh.MarshalAuthorizedKey(userPubKey.PublicKey()))

	if err := client.RunCommand(fmt.Sprintf("mkdir -p /home/%s/.ssh", newUser)); err != nil {
		fmt.Printf("Failed to create .ssh directory: %v\n", err)
		return err
	}

	if err := client.RunCommand(fmt.Sprintf("echo '%s' >> /home/%s/.ssh/authorized_keys", userPubKeyString, newUser)); err != nil {
		fmt.Printf("Failed to add SSH key: %v\n", err)
		return err
	}
	fmt.Println("SSH key setup completed.")
	return nil
}

func installSoftware(client *ssh.Client) error {
	fmt.Println("Installing essential software and Docker...")
	commands := []string{
		"apt-get update",
		"apt-get install -y apt-transport-https ca-certificates curl wget git software-properties-common",
		"curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -",
		"add-apt-repository \"deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable\"",
		"apt-get update",
		"apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin",
	}

	for _, cmd := range commands {
		fmt.Printf("Running: %s", cmd)
		if err := client.RunCommandWithProgress(cmd); err != nil {
			fmt.Printf("\nCommand failed: %v\n", err)
			return err
		}
	}
	fmt.Println("Essential software and Docker installed successfully.")

	fmt.Println("Checking Docker Compose installation...")
	if err := client.RunCommand("docker compose version"); err != nil {
		fmt.Println("Docker Compose not found. Installing...")
		composeCommands := []string{
			"curl -L \"https://github.com/docker/compose/releases/download/v2.17.2/docker-compose-$(uname -s)-$(uname -m)\" -o /usr/local/bin/docker-compose",
			"chmod +x /usr/local/bin/docker-compose",
		}
		for _, cmd := range composeCommands {
			fmt.Printf("Running: %s", cmd)
			if err := client.RunCommandWithProgress(cmd); err != nil {
				fmt.Printf("\nFailed to install Docker Compose: %v\n", err)
				return err
			}
		}
		fmt.Println("Docker Compose installed successfully.")
	} else {
		fmt.Println("Docker Compose is already installed.")
	}
	return nil
}

func configureUFW(client *ssh.Client) error {
	fmt.Println("Installing and configuring UFW...")
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
		fmt.Printf("Running: %s", cmd)
		if err := client.RunCommandWithProgress(cmd); err != nil {
			fmt.Printf("\nFailed to run UFW command: %v\n", err)
			return err
		}
	}
	fmt.Println("UFW installed and configured successfully.")
	return nil
}
