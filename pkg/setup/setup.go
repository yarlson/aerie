package setup

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/yarlson/ftl/pkg/config"
	"github.com/yarlson/ftl/pkg/console"
	"github.com/yarlson/ftl/pkg/executor/local"
	sshPkg "github.com/yarlson/ftl/pkg/executor/ssh"
)

func DockerLogin(ctx context.Context, dockerUsername, dockerPassword string) error {
	executor := local.NewExecutor()

	if err := executor.RunCommandWithProgress(
		ctx,
		"Logging into Docker Hub...",
		"Logged into Docker Hub successfully.",
		[]string{
			fmt.Sprintf("docker login -u %s -p %s", dockerUsername, dockerPassword),
		},
	); err != nil {
		return fmt.Errorf("failed to configure docker hub: %w", err)
	}

	return nil
}

func RunSetup(ctx context.Context, server config.Server, sshKeyPath, dockerUsername, dockerPassword, newUserPassword string) error {
	client, rootKey, err := sshPkg.FindKeyAndConnectWithUser(server.Host, server.Port, "root", sshKeyPath)
	if err != nil {
		return fmt.Errorf("failed to find a suitable SSH key and connect to the server: %w", err)
	}
	defer client.Close()
	console.Success("SSH connection to the server established.")

	if err := installServerSoftware(ctx, client); err != nil {
		return err
	}

	if err := configureServerFirewall(ctx, client); err != nil {
		return err
	}

	if err := createServerUser(ctx, client, server.User, newUserPassword); err != nil {
		return err
	}

	if err := setupServerSSHKey(ctx, client, server.User, rootKey); err != nil {
		return err
	}

	if dockerUsername != "" && dockerPassword != "" {
		client, _, err = sshPkg.FindKeyAndConnectWithUser(server.Host, server.Port, server.User, sshKeyPath)
		if err != nil {
			return fmt.Errorf("failed to find a suitable SSH key and connect to the server: %w", err)
		}
		defer client.Close()

		if err := configureDockerHub(ctx, client, dockerUsername, dockerPassword); err != nil {
			return fmt.Errorf("failed to configure docker hub: %w", err)
		}
	}

	return nil
}

func installServerSoftware(ctx context.Context, client *sshPkg.Client) error {
	commands := []string{
		"apt-get update",
		"apt-get install -y apt-transport-https ca-certificates curl wget git software-properties-common",
		"curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -",
		"add-apt-repository \"deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable\" -y",
		"apt-get update",
		"apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin",
	}

	return client.RunCommandWithProgress(
		ctx,
		"Provisioning server with essential software...",
		"Essential software and Docker installed successfully.",
		commands,
	)
}

func configureServerFirewall(ctx context.Context, client *sshPkg.Client) error {
	commands := []string{
		"apt-get install -y ufw",
		"ufw default deny incoming",
		"ufw default allow outgoing",
		"ufw allow 22/tcp",
		"ufw allow 80/tcp",
		"ufw allow 443/tcp",
		"echo 'y' | ufw enable",
	}

	return client.RunCommandWithProgress(
		ctx,
		"Configuring server firewall...",
		"Server firewall configured successfully.",
		commands,
	)
}

func createServerUser(ctx context.Context, client *sshPkg.Client, newUser, password string) error {
	checkUserCmd := fmt.Sprintf("id -u %s > /dev/null 2>&1", newUser)
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := client.RunCommand(checkCtx, checkUserCmd)
	if err == nil {
		console.Warning(fmt.Sprintf("User %s already exists. Skipping user creation.", newUser))
	} else {
		commands := []string{
			fmt.Sprintf("adduser --gecos '' --disabled-password %s", newUser),
			fmt.Sprintf("echo '%s:%s' | chpasswd", newUser, password),
		}

		err := client.RunCommandWithProgress(
			ctx,
			fmt.Sprintf("Creating user %s...", newUser),
			fmt.Sprintf("User %s created successfully.", newUser),
			commands,
		)
		if err != nil {
			return err
		}
	}

	addToDockerCmd := fmt.Sprintf("usermod -aG docker %s", newUser)
	return client.RunCommandWithProgress(
		ctx,
		fmt.Sprintf("Adding user %s to Docker group...", newUser),
		fmt.Sprintf("User %s added to Docker group successfully.", newUser),
		[]string{addToDockerCmd},
	)
}

func setupServerSSHKey(ctx context.Context, client *sshPkg.Client, newUser string, userKey []byte) error {
	userPubKey, err := ssh.ParsePrivateKey(userKey)
	if err != nil {
		return fmt.Errorf("failed to parse user private key for server access: %w", err)
	}
	userPubKeyString := string(ssh.MarshalAuthorizedKey(userPubKey.PublicKey()))

	commands := []string{
		fmt.Sprintf("mkdir -p /home/%s/.ssh", newUser),
		fmt.Sprintf("echo '%s' | tee -a /home/%s/.ssh/authorized_keys", userPubKeyString, newUser),
		fmt.Sprintf("chown -R %s:%s /home/%s/.ssh", newUser, newUser, newUser),
		fmt.Sprintf("chmod 700 /home/%s/.ssh", newUser),
		fmt.Sprintf("chmod 600 /home/%s/.ssh/authorized_keys", newUser),
	}

	return client.RunCommandWithProgress(
		ctx,
		"Configuring SSH access for the new user...",
		"SSH access configured successfully.",
		commands,
	)
}

func configureDockerHub(ctx context.Context, client *sshPkg.Client, dockerUsername, dockerPassword string) error {
	commands := []string{
		fmt.Sprintf("docker login -u %s -p %s", dockerUsername, dockerPassword),
	}

	return client.RunCommandWithProgress(
		ctx,
		"Logging into Docker Hub...",
		"Logged into Docker Hub successfully.",
		commands,
	)
}
