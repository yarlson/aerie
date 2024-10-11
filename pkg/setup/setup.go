package setup

import (
	"fmt"
	"syscall"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	gssh "golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/yarlson/aerie/pkg/ssh"
)

var (
	info       = color.New(color.FgCyan).PrintlnFunc()
	success    = color.New(color.FgGreen).PrintlnFunc()
	warning    = color.New(color.FgYellow).PrintlnFunc()
	errPrintln = color.New(color.FgRed).PrintlnFunc()
)

func RunSetup(cmd *cobra.Command, args []string) {
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")
	defaultUser, _ := cmd.Flags().GetString("default-user")
	newUser, _ := cmd.Flags().GetString("user")
	sshKeyPath, _ := cmd.Flags().GetString("ssh-key")
	rootKeyPath, _ := cmd.Flags().GetString("root-key")

	info("Starting server provisioning process...")

	// Establish SSH connection to the server
	client, rootKey, e := ssh.FindKeyAndConnectWithUser(host, port, defaultUser, rootKeyPath)
	if e != nil {
		errPrintln("Failed to find a suitable SSH key and connect to the server:", e)
		return
	}
	defer client.Close()
	success("SSH connection to the server established.")

	userKey, e := ssh.FindSSHKey(sshKeyPath)
	if e != nil {
		warning("Failed to find user SSH key. Will use root key for the new user on the server.")
		userKey = rootKey
	} else {
		success("User SSH key for server access found.")
	}

	// Prompt for new user password
	fmt.Print("Enter password for new server user: ")
	newUserPassword, e := term.ReadPassword(syscall.Stdin)
	if e != nil {
		errPrintln("\nFailed to read new server user password:", e)
		return
	}
	success("\nServer user password received.")

	// Perform provisioning steps
	if e := installServerSoftware(client); e != nil {
		return
	}

	if e := configureServerFirewall(client); e != nil {
		return
	}

	if e := createServerUser(client, newUser, string(newUserPassword)); e != nil {
		return
	}

	if e := setupServerSSHKey(client, newUser, userKey); e != nil {
		return
	}

	success("Server provisioning completed successfully!")
}

func createServerUser(client *ssh.Client, newUser, password string) error {
	commands := []string{
		fmt.Sprintf("sudo adduser --gecos '' --disabled-password %s", newUser),
		fmt.Sprintf("echo '%s:%s' | sudo chpasswd", newUser, password),
		fmt.Sprintf("sudo usermod -aG docker %s", newUser),
	}

	if e := client.RunCommandWithProgress(
		fmt.Sprintf("Creating user %s and adding to Docker group...", newUser),
		fmt.Sprintf("User %s created and added to Docker group successfully.", newUser),
		commands,
	); e != nil {
		errPrintln("Failed to create user or add to Docker group on the server:", e)
		return e
	}

	return nil
}

func setupServerSSHKey(client *ssh.Client, newUser string, userKey []byte) error {
	userPubKey, e := gssh.ParsePrivateKey(userKey)
	if e != nil {
		errPrintln("Failed to parse user private key for server access:", e)
		return e
	}
	userPubKeyString := string(gssh.MarshalAuthorizedKey(userPubKey.PublicKey()))

	commands := []string{
		fmt.Sprintf("sudo mkdir -p /home/%s/.ssh", newUser),
		fmt.Sprintf("echo '%s' | sudo tee -a /home/%s/.ssh/authorized_keys", userPubKeyString, newUser),
		fmt.Sprintf("sudo chown -R %s:%s /home/%s/.ssh", newUser, newUser, newUser),
		fmt.Sprintf("sudo chmod 700 /home/%s/.ssh", newUser),
		fmt.Sprintf("sudo chmod 600 /home/%s/.ssh/authorized_keys", newUser),
	}

	if e := client.RunCommandWithProgress(
		"Configuring SSH access for the new user...",
		"SSH access configured successfully.",
		commands,
	); e != nil {
		errPrintln("Failed to set up SSH key on the server:", e)
		return e
	}

	return nil
}

func installServerSoftware(client *ssh.Client) error {
	commands := []string{
		"sudo apt-get update",
		"sudo apt-get install -y apt-transport-https ca-certificates curl wget git software-properties-common",
		"curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -",
		"sudo add-apt-repository \"deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable\" -y",
		"sudo apt-get update",
		"sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin",
	}

	if e := client.RunCommandWithProgress(
		"Provisioning server with essential software and Docker...",
		"Essential software and Docker installed successfully.",
		commands,
	); e != nil {
		errPrintln("Failed to provision server with software:", e)
		return e
	}

	return nil
}

func configureServerFirewall(client *ssh.Client) error {
	commands := []string{
		"sudo apt-get install -y ufw",
		"sudo ufw default deny incoming",
		"sudo ufw default allow outgoing",
		"sudo ufw allow 22/tcp",
		"sudo ufw allow 80/tcp",
		"sudo ufw allow 443/tcp",
		"echo 'y' | sudo ufw enable",
	}

	if e := client.RunCommandWithProgress(
		"Configuring server firewall (UFW)...",
		"Server firewall configured successfully.",
		commands,
	); e != nil {
		errPrintln("Failed to configure server firewall:", e)
		return e
	}

	return nil
}
