package deployment

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"io"
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
