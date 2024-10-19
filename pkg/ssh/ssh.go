package ssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bramvdbogaerde/go-scp"
	"golang.org/x/crypto/ssh"

	"github.com/yarlson/ftl/pkg/logfmt"
)

type Client struct {
	sshClient *ssh.Client
	config    *ssh.ClientConfig
	addr      string
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

	addr := fmt.Sprintf("%s:%d", host, port)

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}

	return &Client{
		sshClient: client,
		config:    config,
		addr:      addr,
	}, nil
}

func (c *Client) ensureConnected() error {
	if c.sshClient != nil {
		_, _, err := c.sshClient.SendRequest("keepalive@golang.org", true, nil)
		if err == nil {
			return nil
		}
	}

	var err error
	for i := 0; i < 3; i++ {
		c.sshClient, err = ssh.Dial("tcp", c.addr, c.config)
		if err == nil {
			return nil
		}
		time.Sleep(time.Second * time.Duration(i+1))
	}
	return fmt.Errorf("failed to re-establish SSH connection after 3 attempts")
}

func (c *Client) Close() error {
	if c.sshClient == nil {
		return nil
	}

	err := c.sshClient.Close()
	c.sshClient = nil
	return err
}

func (c *Client) RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error) {
	if err := c.ensureConnected(); err != nil {
		return nil, err
	}

	session, err := c.sshClient.NewSession()
	if err != nil {
		return nil, fmt.Errorf("unable to create session: %v", err)
	}
	defer session.Close()

	fullCommand := command
	if len(args) > 0 {
		var quotedArgs []string
		for _, arg := range args {
			quotedArgs = append(quotedArgs, fmt.Sprintf("%q", arg))
		}
		fullCommand += " " + strings.Join(quotedArgs, " ")
	}

	pr, pw := io.Pipe()

	session.Stdout = pw
	session.Stderr = pw

	if err := session.Start(fullCommand); err != nil {
		pw.Close()
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
		pw.Close()
	}()

	var output bytes.Buffer
	outputChan := make(chan struct{})

	go func() {
		_, _ = io.Copy(&output, pr)
		close(outputChan)
	}()

	var commandErr error
	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGTERM)
		commandErr = ctx.Err()
	case err := <-done:
		if err != nil {
			commandErr = fmt.Errorf("command failed: %w", err)
		}
	}

	<-outputChan

	if commandErr != nil {
		return bytes.NewReader(output.Bytes()), commandErr
	}

	return bytes.NewReader(output.Bytes()), nil
}

func (c *Client) CopyFile(ctx context.Context, src, dst string) error {
	if err := c.ensureConnected(); err != nil {
		return err
	}

	client, err := scp.NewClientBySSH(c.sshClient)
	if err != nil {
		return fmt.Errorf("failed to create SCP client: %w", err)
	}

	file, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	return client.CopyFile(ctx, file, dst, "0644")
}

func (c *Client) RunCommandWithProgress(ctx context.Context, initialMsg, completeMsg string, commands []string) error {
	operations := make([]func() error, len(commands))
	for i, command := range commands {
		cmd := command // Create a new variable to avoid closure issues
		operations[i] = func() error {
			return c.runSingleCommand(ctx, cmd)
		}
	}

	return logfmt.ProgressSpinner(ctx, initialMsg, completeMsg, operations)
}

func (c *Client) runSingleCommand(ctx context.Context, command string) error {
	session, err := c.sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("unable to create session: %w", err)
	}
	defer session.Close()

	pr, pw := io.Pipe()
	defer pr.Close()

	session.Stdout = pw
	session.Stderr = pw

	if err := session.Start(command); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
		pw.Close()
	}()

	var output strings.Builder

	go func() {
		_, _ = io.Copy(&output, pr)
	}()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGTERM)
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%w\nOutput: %s", err, output.String())
		}
		return nil
	}
}

func (c *Client) RunCommandOutput(command string) (string, error) {
	session, err := c.sshClient.NewSession()
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
