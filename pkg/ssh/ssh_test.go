package ssh

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/crypto/ssh"
)

func TestSSHClient(t *testing.T) {
	ctx := context.Background()

	// Generate SSH key pair
	privateKey, publicKey, err := generateSSHKeyPair()
	assert.NoError(t, err)

	// Start an SSH server container
	req := testcontainers.ContainerRequest{
		Image:        "linuxserver/openssh-server",
		ExposedPorts: []string{"2222/tcp"},
		Env: map[string]string{
			"PUID":            "1000",
			"PGID":            "1000",
			"PUBLIC_KEY":      publicKey,
			"SUDO_ACCESS":     "true",
			"PASSWORD_ACCESS": "false",
			"USER_NAME":       "abc",
		},
		WaitingFor: wait.ForListeningPort("2222/tcp").WithStartupTimeout(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	assert.NoError(t, err)
	defer func() {
		err := container.Terminate(ctx)
		assert.NoError(t, err)
	}()

	// Get the host and port
	host, err := container.Host(ctx)
	assert.NoError(t, err)
	mappedPort, err := container.MappedPort(ctx, "2222")
	assert.NoError(t, err)
	port := mappedPort.Port()

	// Read the private key
	key := []byte(privateKey)

	// Wait for the SSH server to start
	time.Sleep(5 * time.Second)

	// Connect to the SSH server
	client, err := ConnectWithUser(host+":"+port, "abc", key)
	assert.NoError(t, err)
	defer client.Close()

	// Run a command
	_, err = client.RunCommand("echo 'Hello, World!'")
	assert.NoError(t, err)

	// Upload a file
	localFilePath := "test_upload.txt"
	remoteFilePath := "/tmp/test_upload.txt"

	err = os.WriteFile(localFilePath, []byte("test data"), 0644)
	assert.NoError(t, err)
	defer os.Remove(localFilePath)

	err = client.UploadFile(localFilePath, remoteFilePath)
	assert.NoError(t, err)

	// Verify the file was uploaded
	session, err := client.NewSession()
	assert.NoError(t, err)
	defer session.Close()

	output, err := session.CombinedOutput("cat " + remoteFilePath)
	assert.NoError(t, err)
	assert.Equal(t, "test data", string(output))
}

func generateSSHKeyPair() (privateKey string, publicKey string, err error) {
	// Generate the key pair
	privateKeyObj, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKeyObj)
	privateKeyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	publicKeyObj, err := ssh.NewPublicKey(&privateKeyObj.PublicKey)
	if err != nil {
		return "", "", err
	}
	publicKeyBytes := ssh.MarshalAuthorizedKey(publicKeyObj)

	return string(privateKeyPem), string(publicKeyBytes), nil
}
