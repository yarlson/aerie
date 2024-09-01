package setup

import (
	"fmt"
	"syscall"

	"github.com/enclave-ci/aerie/internal/ssh"
	"github.com/enclave-ci/aerie/pkg/utils"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	gssh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

var (
	info    = color.New(color.FgCyan).PrintlnFunc()
	success = color.New(color.FgGreen).PrintlnFunc()
	warning = color.New(color.FgYellow).PrintlnFunc()
	err     = color.New(color.FgRed).PrintlnFunc()
)

func RunSetup(cmd *cobra.Command, args []string) {
	host, _ := cmd.Flags().GetString("host")
	newUser, _ := cmd.Flags().GetString("user")
	sshKeyPath, _ := cmd.Flags().GetString("ssh-key")
	rootKeyPath, _ := cmd.Flags().GetString("root-key")

	info("Starting server setup process...")

	// Find appropriate SSH keys
	info("Locating SSH keys...")
	rootKey, e := utils.FindSSHKey(rootKeyPath, true)
	if e != nil {
		err("Failed to find root SSH key:", e)
		return
	}
	success("Root SSH key found.")

	userKey, e := utils.FindSSHKey(sshKeyPath, false)
	if e != nil {
		warning("Failed to find user SSH key, using root key for new user")
		userKey = rootKey
	} else {
		success("User SSH key found.")
	}

	// Prompt for new user password
	fmt.Print("Enter password for new user: ")
	newUserPassword, e := term.ReadPassword(int(syscall.Stdin))
	if e != nil {
		err("\nFailed to read new user password:", e)
		return
	}
	success("\nPassword received.")

	// Connect to SSH
	client, e := ssh.Connect(host, rootKey)
	if e != nil {
		err("Failed to connect:", e)
		return
	}
	defer client.Close()

	// Perform setup steps
	if e := createUser(client, newUser, string(newUserPassword)); e != nil {
		return
	}

	if e := setupSSHKey(client, newUser, userKey); e != nil {
		return
	}

	if e := installSoftware(client); e != nil {
		return
	}

	if e := configureUFW(client); e != nil {
		return
	}

	success("Setup completed successfully!")
}

func createUser(client *ssh.Client, newUser, password string) error {
	info("Creating new user:", newUser)
	if e := client.RunCommand(fmt.Sprintf("adduser --gecos '' --disabled-password %s", newUser)); e != nil {
		err("Failed to create user:", e)
		return e
	}

	if e := client.RunCommand(fmt.Sprintf("echo '%s:%s' | chpasswd", newUser, password)); e != nil {
		err("Failed to set user password:", e)
		return e
	}
	success("User created successfully.")
	return nil
}

func setupSSHKey(client *ssh.Client, newUser string, userKey []byte) error {
	info("Setting up SSH key for new user...")
	userPubKey, e := gssh.ParsePrivateKey(userKey)
	if e != nil {
		err("Failed to parse user private key:", e)
		return e
	}
	userPubKeyString := string(gssh.MarshalAuthorizedKey(userPubKey.PublicKey()))

	if e := client.RunCommand(fmt.Sprintf("mkdir -p /home/%s/.ssh", newUser)); e != nil {
		err("Failed to create .ssh directory:", e)
		return e
	}

	if e := client.RunCommand(fmt.Sprintf("echo '%s' >> /home/%s/.ssh/authorized_keys", userPubKeyString, newUser)); e != nil {
		err("Failed to add SSH key:", e)
		return e
	}
	success("SSH key setup completed.")
	return nil
}

func installSoftware(client *ssh.Client) error {
	info("Installing essential software and Docker...")
	commands := []string{
		"apt-get update",
		"apt-get install -y apt-transport-https ca-certificates curl wget git software-properties-common",
		"curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -",
		"add-apt-repository \"deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable\"",
		"apt-get update",
		"apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin",
	}

	for _, cmd := range commands {
		info("Running:", cmd)
		if e := client.RunCommandWithProgress(cmd); e != nil {
			err("\nCommand failed:", e)
			return e
		}
	}
	success("Essential software and Docker installed successfully.")

	info("Checking Docker Compose installation...")
	if e := client.RunCommand("docker compose version"); e != nil {
		warning("Docker Compose not found. Installing...")
		composeCommands := []string{
			"curl -L \"https://github.com/docker/compose/releases/download/v2.17.2/docker-compose-$(uname -s)-$(uname -m)\" -o /usr/local/bin/docker-compose",
			"chmod +x /usr/local/bin/docker-compose",
		}
		for _, cmd := range composeCommands {
			info("Running:", cmd)
			if e := client.RunCommandWithProgress(cmd); e != nil {
				err("\nFailed to install Docker Compose:", e)
				return e
			}
		}
		success("Docker Compose installed successfully.")
	} else {
		success("Docker Compose is already installed.")
	}
	return nil
}

func configureUFW(client *ssh.Client) error {
	info("Installing and configuring UFW...")
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
		info("Running:", cmd)
		if e := client.RunCommandWithProgress(cmd); e != nil {
			err("\nFailed to run UFW command:", e)
			return e
		}
	}
	success("UFW installed and configured successfully.")
	return nil
}
