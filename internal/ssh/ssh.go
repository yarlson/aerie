package ssh

import (
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	*ssh.Client
}

func Connect(host string, key []byte) (*Client, error) {
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}

	return &Client{client}, nil
}

func (c *Client) RunCommand(command string) error {
	session, err := c.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return fmt.Errorf("command failed: %v\nOutput: %s", err, string(output))
	}

	return nil
}

func (c *Client) RunCommandWithProgress(command string) error {
	session, err := c.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	done := make(chan bool)
	go func() {
		fmt.Print("\n")
		for {
			select {
			case <-done:
				fmt.Print("\n")
				return
			default:
				fmt.Print(".")
				time.Sleep(time.Second)
			}
		}
	}()

	output, err := session.CombinedOutput(command)
	done <- true

	if err != nil {
		return fmt.Errorf("command failed: %v\nOutput: %s", err, string(output))
	}

	return nil
}
