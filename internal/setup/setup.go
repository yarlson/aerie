package setup

import (
	"fmt"
	"syscall"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	gssh "golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/enclave-ci/aerie/internal/ssh"
	"github.com/enclave-ci/aerie/pkg/utils"
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

	info("Starting server provisioning process...")

	// Find appropriate SSH keys
	info("Locating SSH keys for server access...")
	rootKey, e := utils.FindSSHKey(rootKeyPath, true)
	if e != nil {
		err("Failed to find root SSH key for server access:", e)
		return
	}
	success("Root SSH key for server access found.")

	userKey, e := utils.FindSSHKey(sshKeyPath, false)
	if e != nil {
		warning("Failed to find user SSH key, will use root key for new user on the server")
		userKey = rootKey
	} else {
		success("User SSH key for server access found.")
	}

	// Prompt for new user password
	fmt.Print("Enter password for new server user: ")
	newUserPassword, e := term.ReadPassword(int(syscall.Stdin))
	if e != nil {
		err("\nFailed to read new server user password:", e)
		return
	}
	success("\nServer user password received.")

	// Connect to SSH
	info("Establishing SSH connection to the server...")
	client, e := ssh.Connect(host, rootKey)
	if e != nil {
		err("Failed to connect to the server:", e)
		return
	}
	defer client.Close()
	success("SSH connection to the server established.")

	// Perform provisioning steps
	if e := createServerUser(client, newUser, string(newUserPassword)); e != nil {
		return
	}

	if e := setupServerSSHKey(client, newUser, userKey); e != nil {
		return
	}

	if e := installServerSoftware(client); e != nil {
		return
	}

	if e := configureServerFirewall(client); e != nil {
		return
	}

	success("Server provisioning completed successfully!")
}

func createServerUser(client *ssh.Client, newUser, password string) error {
	info("Creating new user on the server:", newUser)
	if e := client.RunCommand(fmt.Sprintf("adduser --gecos '' --disabled-password %s", newUser)); e != nil {
		err("Failed to create user on the server:", e)
		return e
	}

	if e := client.RunCommand(fmt.Sprintf("echo '%s:%s' | chpasswd", newUser, password)); e != nil {
		err("Failed to set user password on the server:", e)
		return e
	}
	success("Server user created successfully.")
	return nil
}

func setupServerSSHKey(client *ssh.Client, newUser string, userKey []byte) error {
	info("Setting up SSH key for new user on the server...")
	userPubKey, e := gssh.ParsePrivateKey(userKey)
	if e != nil {
		err("Failed to parse user private key for server access:", e)
		return e
	}
	userPubKeyString := string(gssh.MarshalAuthorizedKey(userPubKey.PublicKey()))

	if e := client.RunCommand(fmt.Sprintf("mkdir -p /home/%s/.ssh", newUser)); e != nil {
		err("Failed to create .ssh directory on the server:", e)
		return e
	}

	if e := client.RunCommand(fmt.Sprintf("echo '%s' >> /home/%s/.ssh/authorized_keys", userPubKeyString, newUser)); e != nil {
		err("Failed to add SSH key to the server:", e)
		return e
	}
	success("SSH key setup on the server completed.")
	return nil
}

func installServerSoftware(client *ssh.Client) error {
	info("Provisioning server with essential software and Docker...")
	commands := []string{
		"apt-get update",
		"apt-get install -y apt-transport-https ca-certificates curl wget git software-properties-common",
		"curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -",
		"add-apt-repository \"deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable\"",
		"apt-get update",
		"apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin",
	}

	for _, cmd := range commands {
		info("Executing server command:", cmd)
		if e := client.RunCommandWithProgress(cmd); e != nil {
			err("\nServer command failed:", e)
			return e
		}
	}
	success("Server provisioned with essential software and Docker successfully.")

	info("Verifying Docker Compose installation on the server...")
	if e := client.RunCommand("docker compose version"); e != nil {
		warning("Docker Compose not found on the server. Installing...")
		composeCommands := []string{
			"curl -L \"https://github.com/docker/compose/releases/download/v2.17.2/docker-compose-$(uname -s)-$(uname -m)\" -o /usr/local/bin/docker-compose",
			"chmod +x /usr/local/bin/docker-compose",
		}
		for _, cmd := range composeCommands {
			info("Executing server command:", cmd)
			if e := client.RunCommandWithProgress(cmd); e != nil {
				err("\nFailed to install Docker Compose on the server:", e)
				return e
			}
		}
		success("Docker Compose installed on the server successfully.")
	} else {
		success("Docker Compose is already installed on the server.")
	}
	return nil
}

func configureServerFirewall(client *ssh.Client) error {
	info("Configuring server firewall (UFW)...")
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
		info("Executing firewall command:", cmd)
		if e := client.RunCommandWithProgress(cmd); e != nil {
			err("\nFailed to configure server firewall:", e)
			return e
		}
	}
	success("Server firewall (UFW) configured successfully.")
	return nil
}
