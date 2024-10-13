package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yarlson/aerie/pkg/config"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type UpdaterTestSuite struct {
	suite.Suite
	ctx       context.Context
	updater   *Updater
	container testcontainers.Container
	network   string
}

func TestUpdaterSuite(t *testing.T) {
	suite.Run(t, new(UpdaterTestSuite))
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

func (suite *UpdaterTestSuite) SetupSuite() {
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

func (suite *UpdaterTestSuite) TearDownSuite() {
	if suite.container != nil {
		suite.Require().NoError(suite.container.Terminate(suite.ctx))
	}
	_ = exec.Command("docker", "network", "rm", suite.network).Run()
}

func (suite *UpdaterTestSuite) SetupTest() {
	executor := &LocalExecutor{}
	suite.updater = NewUpdater(executor)
}

func (suite *UpdaterTestSuite) createTempDir() string {
	tmpDir, err := os.MkdirTemp("", "docker-test")
	assert.NoError(suite.T(), err)
	return tmpDir
}

func (suite *UpdaterTestSuite) createInitialContainer(service, network, tmpDir string) {
	cmd := exec.Command("docker", "run", "-d",
		"--name", service,
		"--network", network,
		"--network-alias", service,
		"--env", "TEST_ENV=test_value",
		"--label", "com.example.label=test_label",
		"--health-cmd", "curl -f http://localhost/ || exit 1",
		"--health-interval=5s",
		"--health-retries=3",
		"--health-timeout=2s",
		"-v", tmpDir+":/container/path",
		"nginx:latest")
	err := cmd.Run()
	assert.NoError(suite.T(), err)
}

func (suite *UpdaterTestSuite) removeContainer(containerName string) {
	_ = exec.Command("docker", "rm", "-f", containerName).Run()
}

func (suite *UpdaterTestSuite) inspectContainer(containerName string) map[string]interface{} {
	cmd := exec.Command("docker", "inspect", containerName)
	output, err := cmd.Output()
	assert.NoError(suite.T(), err)

	var containerInfo []map[string]interface{}
	err = json.Unmarshal(output, &containerInfo)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), containerInfo, 1)

	return containerInfo[0]
}

func (suite *UpdaterTestSuite) TestGetContainerId_Success() {
	service := "nginx"
	network := "aerie-test-network"

	containerID, err := suite.updater.getContainerID(service, network)

	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), containerID)
}

func (suite *UpdaterTestSuite) TestGetContainerId_NoContainerFound() {
	service := "non-existent-service"
	network := "non-existent-network"

	containerID, err := suite.updater.getContainerID(service, network)

	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "no container found with alias non-existent-service in network non-existent-network")
	assert.Empty(suite.T(), containerID)
}

func (suite *UpdaterTestSuite) TestStartNewContainer_Success() {
	tmpDir := suite.createTempDir()
	defer os.RemoveAll(tmpDir)

	service := &config.Service{
		Name:  "nginx",
		Image: "nginx:latest",
		Port:  80,
		EnvVars: []config.EnvVar{
			{
				Name:  "TEST_ENV",
				Value: "test_value",
			},
		},
		Volumes: []string{
			tmpDir + ":/container/path",
		},
		HealthCheck: &config.HealthCheck{
			Path:     "/",
			Interval: time.Second,
			Timeout:  time.Second,
			Retries:  3,
		},
	}

	svsName := service.Name
	network := "aerie-test-network"
	suite.createInitialContainer(svsName, network, tmpDir)
	defer suite.removeContainer(svsName)

	err := suite.updater.startNewContainer(service, network)
	assert.NoError(suite.T(), err)
	defer suite.removeContainer(svsName + newContainerSuffix)

	containerInfo := suite.inspectContainer(svsName + newContainerSuffix)

	suite.Run("Container State and Config", func() {
		assert.Equal(suite.T(), "running", containerInfo["State"].(map[string]interface{})["Status"])
		assert.Contains(suite.T(), containerInfo["Config"].(map[string]interface{})["Image"], "nginx")
		assert.Equal(suite.T(), network, containerInfo["HostConfig"].(map[string]interface{})["NetworkMode"])
	})

	suite.Run("Environment Variables", func() {
		env := containerInfo["Config"].(map[string]interface{})["Env"].([]interface{})
		assert.Contains(suite.T(), env, "TEST_ENV=test_value")
	})

	suite.Run("Volume Bindings", func() {
		binds := containerInfo["HostConfig"].(map[string]interface{})["Binds"].([]interface{})
		assert.Contains(suite.T(), binds, tmpDir+":/container/path")
	})

	suite.Run("Network Aliases", func() {
		networkSettings := containerInfo["NetworkSettings"].(map[string]interface{})
		networks := networkSettings["Networks"].(map[string]interface{})
		networkInfo := networks[network].(map[string]interface{})
		aliases := networkInfo["Aliases"].([]interface{})
		assert.Contains(suite.T(), aliases, svsName+newContainerSuffix)
	})

	suite.Run("Health Checks", func() {
		err = suite.updater.performHealthChecks(svsName+newContainerSuffix, service.HealthCheck)
		assert.NoError(suite.T(), err)
	})

	suite.Run("Switch Traffic", func() {
		err = suite.updater.switchTraffic(svsName, network)
		assert.NoError(suite.T(), err)

		newContainerInfo, err := suite.updater.getContainerInfo(svsName, network)
		assert.NoError(suite.T(), err)

		assert.Contains(suite.T(), newContainerInfo.NetworkSettings.Networks, network)

		networkAliases := newContainerInfo.NetworkSettings.Networks[network].Aliases
		assert.Contains(suite.T(), networkAliases, svsName)

		// Verify old container is no longer in the network
		oldContainerID, err := suite.updater.getContainerID(svsName+newContainerSuffix, network)
		assert.Error(suite.T(), err)
		assert.Empty(suite.T(), oldContainerID)
	})

	suite.Run("Cleanup", func() {
		err := suite.updater.cleanup(svsName, network)
		assert.NoError(suite.T(), err)

		output, err := suite.updater.runCommand(context.Background(), "docker", "ps", "-a", "--filter", fmt.Sprintf("name=%s", svsName))
		assert.NoError(suite.T(), err)
		assert.NotContains(suite.T(), output, svsName+newContainerSuffix)

		assert.Contains(suite.T(), output, svsName)
	})
}
