package ssh

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Client struct {
	*ssh.Client
}

func ConnectWithUser(host, user string, key []byte) (*Client, error) {
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

	client, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}

	return &Client{client}, nil
}

func (c *Client) RunCommand(command string) ([]byte, error) {
	session, err := c.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return nil, fmt.Errorf("command failed: %v\nOutput: %s", err, string(output))
	}

	return output, nil
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

func (c *Client) UploadFile(localPath, remotePath string) error {
	sftpClient, err := sftp.NewClient(c.Client)
	if err != nil {
		return fmt.Errorf("failed to create sftp client: %v", err)
	}
	defer sftpClient.Close()

	srcFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %v", err)
	}
	defer srcFile.Close()

	dstFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %v", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file: %v", err)
	}

	return nil
}
