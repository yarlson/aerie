package setup

import (
	"fmt"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/yarlson/aerie/pkg/config"
	"github.com/yarlson/aerie/pkg/logfmt"
	sshPkg "github.com/yarlson/aerie/pkg/ssh"
)

func RunSetup(server config.Server, sshKeyPath string) error {
	client, rootKey, err := sshPkg.FindKeyAndConnectWithUser(server.Host, server.Port, server.User, sshKeyPath)
	if err != nil {
		return fmt.Errorf("failed to find a suitable SSH key and connect to the server: %w", err)
	}
	defer client.Close()
	logfmt.Success("SSH connection to the server established.")

	fmt.Print("Enter password for new server user: ")
	newUserPassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("failed to read new server user password: %w", err)
	}
	logfmt.Success("\nServer user password received.")

	if err := installServerSoftware(client); err != nil {
		return err
	}

	if err := configureServerFirewall(client); err != nil {
		return err
	}

	if err := createServerUser(client, server.User, string(newUserPassword)); err != nil {
		return err
	}

	if err := setupServerSSHKey(client, server.User, rootKey); err != nil {
		return err
	}

	logfmt.Success("Server provisioning completed successfully!")
	return nil
}

func installServerSoftware(client *sshPkg.Client) error {
	commands := []string{
		"sudo apt-get update",
		"sudo apt-get install -y apt-transport-https ca-certificates curl wget git software-properties-common",
		"curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -",
		"sudo add-apt-repository \"deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable\" -y",
		"sudo apt-get update",
		"sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin",
	}

	return client.RunCommandWithProgress(
		"Provisioning server with essential software and Docker...",
		"Essential software and Docker installed successfully.",
		commands,
	)
}

func configureServerFirewall(client *sshPkg.Client) error {
	commands := []string{
		"sudo apt-get install -y ufw",
		"sudo ufw default deny incoming",
		"sudo ufw default allow outgoing",
		"sudo ufw allow 22/tcp",
		"sudo ufw allow 80/tcp",
		"sudo ufw allow 443/tcp",
		"echo 'y' | sudo ufw enable",
	}

	return client.RunCommandWithProgress(
		"Configuring server firewall (UFW)...",
		"Server firewall configured successfully.",
		commands,
	)
}

func createServerUser(client *sshPkg.Client, newUser, password string) error {
	commands := []string{
		fmt.Sprintf("sudo adduser --gecos '' --disabled-password %s", newUser),
		fmt.Sprintf("echo '%s:%s' | sudo chpasswd", newUser, password),
		fmt.Sprintf("sudo usermod -aG docker %s", newUser),
	}

	return client.RunCommandWithProgress(
		fmt.Sprintf("Creating user %s and adding to Docker group...", newUser),
		fmt.Sprintf("User %s created and added to Docker group successfully.", newUser),
		commands,
	)
}

func setupServerSSHKey(client *sshPkg.Client, newUser string, userKey []byte) error {
	userPubKey, err := ssh.ParsePrivateKey(userKey)
	if err != nil {
		return fmt.Errorf("failed to parse user private key for server access: %w", err)
	}
	userPubKeyString := string(ssh.MarshalAuthorizedKey(userPubKey.PublicKey()))

	commands := []string{
		fmt.Sprintf("sudo mkdir -p /home/%s/.ssh", newUser),
		fmt.Sprintf("echo '%s' | sudo tee -a /home/%s/.ssh/authorized_keys", userPubKeyString, newUser),
		fmt.Sprintf("sudo chown -R %s:%s /home/%s/.ssh", newUser, newUser, newUser),
		fmt.Sprintf("sudo chmod 700 /home/%s/.ssh", newUser),
		fmt.Sprintf("sudo chmod 600 /home/%s/.ssh/authorized_keys", newUser),
	}

	return client.RunCommandWithProgress(
		"Configuring SSH access for the new user...",
		"SSH access configured successfully.",
		commands,
	)
}
