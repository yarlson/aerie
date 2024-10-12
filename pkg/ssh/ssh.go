package ssh

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"golang.org/x/crypto/ssh"
)

type Client struct {
	*ssh.Client
}

func ConnectWithUser(host string, port int, user string, key []byte) (*Client, error) {
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}

	return &Client{client}, nil
}

func (c *Client) RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error) {
	session, err := c.NewSession()
	if err != nil {
		return nil, fmt.Errorf("unable to create session: %v", err)
	}

	fullCommand := command
	if len(args) > 0 {
		fullCommand += " " + strings.Join(args, " ")
	}

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		defer session.Close()

		session.Stdout = pw
		session.Stderr = pw

		if err := session.Start(fullCommand); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("failed to start command: %v", err))
			return
		}

		done := make(chan error, 1)
		go func() {
			done <- session.Wait()
		}()

		select {
		case <-ctx.Done():
			_ = session.Signal(ssh.SIGTERM)
			_ = pw.CloseWithError(ctx.Err())
		case err := <-done:
			if err != nil {
				_ = pw.CloseWithError(fmt.Errorf("command failed: %v", err))
			}
		}
	}()

	return pr, nil
}

func (c *Client) RunCommandWithProgress(initialMsg, completeMsg string, commands []string) error {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond) // Braille spinner
	s.Suffix = " " + initialMsg
	_ = s.Color("yellow")
	s.Start()

	for _, command := range commands {
		session, err := c.NewSession()
		if err != nil {
			s.Stop()
			return err
		}

		output, err := session.CombinedOutput(command)
		session.Close()
		if err != nil {
			s.Stop()

			errorMessage := fmt.Sprintf(
				"\n%s Command failed: %s\n%s %v\n%s\n%s",
				color.New(color.FgRed).SprintFunc()("X"),
				color.New(color.FgYellow).SprintFunc()(command),
				color.New(color.FgRed).SprintFunc()("Error:"),
				err,
				color.New(color.FgRed).SprintFunc()("Output:"),
				string(output),
			)

			fmt.Println(errorMessage)

			return fmt.Errorf("command failed: %v", err)
		}
	}

	s.Stop()

	checkMark := color.New(color.FgGreen).SprintFunc()("√")
	fmt.Printf("%s %s\n", checkMark, completeMsg)

	return nil
}

func (c *Client) RunCommandOutput(command string) (string, error) {
	session, err := c.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return "", fmt.Errorf("command failed: %v\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

// sshKeyPath is used only for testing purposes
var sshKeyPath string

// FindSSHKey looks for an SSH key in the given path or in default locations
func FindSSHKey(keyPath string) ([]byte, error) {
	if keyPath != "" {
		return os.ReadFile(keyPath)
	}

	sshDir, err := getSSHDir()
	if err != nil {
		return nil, err
	}

	keyNames := []string{"id_rsa", "id_ecdsa", "id_ed25519"}
	for _, name := range keyNames {
		path := filepath.Join(sshDir, name)
		key, err := os.ReadFile(path)
		if err == nil {
			return key, nil
		}
	}

	return nil, fmt.Errorf("no suitable SSH key found in %s", sshDir)
}

// FindKeyAndConnectWithUser finds an SSH key and establishes a connection
func FindKeyAndConnectWithUser(host string, port int, user, keyPath string) (*Client, []byte, error) {
	key, err := FindSSHKey(keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find SSH key: %w", err)
	}

	client, err := ConnectWithUser(host, port, user, key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to establish SSH connection: %w", err)
	}

	return client, key, nil
}

// getSSHDir returns the SSH directory path
func getSSHDir() (string, error) {
	if sshKeyPath != "" {
		return sshKeyPath, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(home, ".ssh"), nil
}
