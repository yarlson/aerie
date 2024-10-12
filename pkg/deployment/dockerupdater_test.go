package deployment

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"io"
	"os"
	"os/exec"
	"testing"
)

type DockerUpdaterTestSuite struct {
	suite.Suite
	ctx       context.Context
	updater   *DockerUpdater
	container testcontainers.Container
	network   string
}

func TestDockerUpdaterSuite(t *testing.T) {
	suite.Run(t, new(DockerUpdaterTestSuite))
}

type LocalExecutor struct{}

func (e *LocalExecutor) RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	combinedOutput, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		_ = cmd.Wait()
	}()

	return combinedOutput, nil
}

func (suite *DockerUpdaterTestSuite) SetupSuite() {
	suite.network = "aerie-test-network"
	_ = exec.Command("docker", "network", "create", suite.network).Run()
	suite.ctx = context.Background()
	req := testcontainers.ContainerRequest{
		Image:          "nginx:latest",
		ExposedPorts:   []string{"80/tcp"},
		WaitingFor:     wait.ForListeningPort("80/tcp"),
		Networks:       []string{suite.network},
		NetworkAliases: map[string][]string{suite.network: {"nginx"}},
	}

	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	suite.Require().NoError(err)

	suite.container = container
}

func (suite *DockerUpdaterTestSuite) TearDownSuite() {
	if suite.container != nil {
		suite.Require().NoError(suite.container.Terminate(suite.ctx))
	}
	_ = exec.Command("docker", "network", "rm", suite.network).Run()
}

func (suite *DockerUpdaterTestSuite) SetupTest() {
	executor := &LocalExecutor{}
	suite.updater = NewDockerUpdater(executor)
}

func (suite *DockerUpdaterTestSuite) TestGetImageName_Success() {
	service := "nginx"
	expectedImage := "nginx:latest"

	imageName, err := suite.updater.getImageName(service, suite.network)

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), expectedImage, imageName)
}

func (suite *DockerUpdaterTestSuite) TestGetImageName_NoContainerFound() {
	service := "non-existent-service"
	network := "non-existent-network"

	imageName, err := suite.updater.getImageName(service, network)

	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "no container found with alias non-existent-service in network non-existent-network")
	assert.Empty(suite.T(), imageName)
}

func (suite *DockerUpdaterTestSuite) TestGetContainerId_Success() {
	service := "nginx"
	network := "aerie-test-network"

	containerID, err := suite.updater.getContainerID(service, network)

	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), containerID)
}

func (suite *DockerUpdaterTestSuite) TestGetContainerId_NoContainerFound() {
	service := "non-existent-service"
	network := "non-existent-network"

	containerID, err := suite.updater.getContainerID(service, network)

	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "no container found with alias non-existent-service in network non-existent-network")
	assert.Empty(suite.T(), containerID)
}

func (suite *DockerUpdaterTestSuite) TestStartNewContainer_Success() {
	service := "nginx"
	network := suite.network

	tmpDir, err := os.MkdirTemp("", "docker-test")
	assert.NoError(suite.T(), err)
	defer os.RemoveAll(tmpDir)

	err = exec.Command("docker", "run", "-d", "--name", service, "--network", network, "--network-alias", service, "--env", "TEST_ENV=test_value", "--label", "com.example.label=test_label", "--health-cmd", "curl -f http://localhost:8080/health || exit 1", "--health-interval=5s", "--health-retries=3", "--health-timeout=2s", "-v", tmpDir+":/container/path", "nginx:latest").Run()
	assert.NoError(suite.T(), err)
	defer func() {
		_ = exec.Command("docker", "rm", "-f", service).Run()
	}()

	err = suite.updater.startNewContainer(service, network)
	assert.NoError(suite.T(), err)
	defer func() {
		_ = exec.Command("docker", "rm", "-f", service+newContainerSuffix).Run()
	}()

	cmd := exec.Command("docker", "inspect", service+newContainerSuffix)
	output, err := cmd.Output()
	assert.NoError(suite.T(), err)

	var containerInfo []struct {
		State struct {
			Status string `json:"Status"`
		} `json:"State"`
		Config struct {
			Image  string            `json:"Image"`
			Env    []string          `json:"Env"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		HostConfig struct {
			NetworkMode string   `json:"NetworkMode"`
			Binds       []string `json:"Binds"`
		} `json:"HostConfig"`
	}

	err = json.Unmarshal(output, &containerInfo)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), containerInfo, 1)

	container := containerInfo[0]
	assert.Equal(suite.T(), "running", container.State.Status)
	assert.Contains(suite.T(), container.Config.Image, "nginx")
	assert.Equal(suite.T(), network, container.HostConfig.NetworkMode)

	assert.Contains(suite.T(), container.Config.Env, "TEST_ENV=test_value")

	assert.Contains(suite.T(), container.HostConfig.Binds, tmpDir+":/container/path")

	assert.Equal(suite.T(), "test_label", container.Config.Labels["com.example.label"])
}

func (suite *DockerUpdaterTestSuite) TestStartNewContainer_NonExistentService() {
	service := "non-existent-service"
	network := suite.network

	err := suite.updater.startNewContainer(service, network)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "failed to get image name")
}

func (suite *DockerUpdaterTestSuite) TestStartNewContainer_NonExistentNetwork() {
	service := "nginx"
	network := "non-existent-network"

	err := suite.updater.startNewContainer(service, network)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "failed to get image name")
}
