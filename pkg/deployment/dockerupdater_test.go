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

func (suite *DockerUpdaterTestSuite) createTempDir() string {
	tmpDir, err := os.MkdirTemp("", "docker-test")
	assert.NoError(suite.T(), err)
	return tmpDir
}

func (suite *DockerUpdaterTestSuite) createInitialContainer(service, network, tmpDir string) {
	cmd := exec.Command("docker", "run", "-d",
		"--name", service,
		"--network", network,
		"--network-alias", service,
		"--env", "TEST_ENV=test_value",
		"--label", "com.example.label=test_label",
		"--health-cmd", "curl -f http://localhost:8080/health || exit 1",
		"--health-interval=5s",
		"--health-retries=3",
		"--health-timeout=2s",
		"-v", tmpDir+":/container/path",
		"nginx:latest")
	err := cmd.Run()
	assert.NoError(suite.T(), err)
}

func (suite *DockerUpdaterTestSuite) removeContainer(containerName string) {
	_ = exec.Command("docker", "rm", "-f", containerName).Run()
}

func (suite *DockerUpdaterTestSuite) inspectContainer(containerName string) map[string]interface{} {
	cmd := exec.Command("docker", "inspect", containerName)
	output, err := cmd.Output()
	assert.NoError(suite.T(), err)

	var containerInfo []map[string]interface{}
	err = json.Unmarshal(output, &containerInfo)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), containerInfo, 1)

	return containerInfo[0]
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
	tmpDir := suite.createTempDir()
	defer os.RemoveAll(tmpDir)

	suite.createInitialContainer(service, network, tmpDir)
	defer suite.removeContainer(service)

	err := suite.updater.startNewContainer(service, network)
	assert.NoError(suite.T(), err)
	defer suite.removeContainer(service + newContainerSuffix)

	containerInfo := suite.inspectContainer(service + newContainerSuffix)

	assert.Equal(suite.T(), "running", containerInfo["State"].(map[string]interface{})["Status"])
	assert.Contains(suite.T(), containerInfo["Config"].(map[string]interface{})["Image"], "nginx")
	assert.Equal(suite.T(), network, containerInfo["HostConfig"].(map[string]interface{})["NetworkMode"])

	env := containerInfo["Config"].(map[string]interface{})["Env"].([]interface{})
	assert.Contains(suite.T(), env, "TEST_ENV=test_value")

	binds := containerInfo["HostConfig"].(map[string]interface{})["Binds"].([]interface{})
	assert.Contains(suite.T(), binds, tmpDir+":/container/path")

	labels := containerInfo["Config"].(map[string]interface{})["Labels"].(map[string]interface{})
	assert.Equal(suite.T(), "test_label", labels["com.example.label"])
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
