package deployment

import (
	"context"
	"encoding/json"
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
	updater   *Deployment
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

func (suite *UpdaterTestSuite) TestInstallService() {
	tmpDir := suite.createTempDir()
	defer os.RemoveAll(tmpDir)

	serviceName := "test-service"
	network := "aerie-test-network"

	service := &config.Service{
		Name:  serviceName,
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
			Retries:  30,
		},
	}

	// Ensure the container doesn't exist before the test
	suite.removeContainer(serviceName)

	// Test InstallService
	err := suite.updater.InstallService(service, network)
	assert.NoError(suite.T(), err)

	defer suite.removeContainer(serviceName)

	// Verify the container was created and is running
	containerInfo := suite.inspectContainer(serviceName)

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
		assert.Contains(suite.T(), aliases, serviceName)
	})

	suite.Run("Health Checks", func() {
		err = suite.updater.performHealthChecks(serviceName, service.HealthCheck)
		assert.NoError(suite.T(), err)
	})
}

func (suite *UpdaterTestSuite) TestUpdateService() {
	tmpDir := suite.createTempDir()
	defer os.RemoveAll(tmpDir)

	serviceName := "test-update-service"
	network := "aerie-test-network"

	initialService := &config.Service{
		Name:  serviceName,
		Image: "nginx:1.19",
		Port:  80,
		EnvVars: []config.EnvVar{
			{
				Name:  "INITIAL_ENV",
				Value: "initial_value",
			},
		},
		Volumes: []string{
			tmpDir + ":/initial/path",
		},
		HealthCheck: &config.HealthCheck{
			Path:     "/",
			Interval: time.Second,
			Timeout:  time.Second,
			Retries:  30,
		},
	}

	suite.removeContainer(serviceName)

	err := suite.updater.InstallService(initialService, network)
	assert.NoError(suite.T(), err)

	defer suite.removeContainer(serviceName)

	initialContainerID, err := suite.updater.getContainerID(serviceName, network)
	assert.NoError(suite.T(), err)

	updatedService := &config.Service{
		Name:  serviceName,
		Image: "nginx:1.20",
		Port:  80,
		EnvVars: []config.EnvVar{
			{
				Name:  "UPDATED_ENV",
				Value: "updated_value",
			},
		},
		Volumes: []string{
			tmpDir + ":/updated/path",
		},
		HealthCheck: &config.HealthCheck{
			Path:     "/",
			Interval: time.Second,
			Timeout:  time.Second,
			Retries:  30,
		},
	}

	err = suite.updater.UpdateService(updatedService, network)
	assert.NoError(suite.T(), err)

	var updatedContainerID string
	for i := 0; i < 30; i++ {
		updatedContainerID, err = suite.updater.getContainerID(serviceName, network)
		if err == nil && updatedContainerID != initialContainerID {
			break
		}
		time.Sleep(time.Second)
	}

	assert.NotEqual(suite.T(), initialContainerID, updatedContainerID)

	containerInfo := suite.inspectContainer(serviceName)

	suite.Run("Updated Container State and Config", func() {
		assert.Equal(suite.T(), "running", containerInfo["State"].(map[string]interface{})["Status"])
		assert.Contains(suite.T(), containerInfo["Config"].(map[string]interface{})["Image"], "nginx:1.20")
		assert.Equal(suite.T(), network, containerInfo["HostConfig"].(map[string]interface{})["NetworkMode"])
	})

	suite.Run("Updated Environment Variables", func() {
		env := containerInfo["Config"].(map[string]interface{})["Env"].([]interface{})
		assert.Contains(suite.T(), env, "UPDATED_ENV=updated_value")
		assert.NotContains(suite.T(), env, "INITIAL_ENV=initial_value")
	})

	suite.Run("Updated Volume Bindings", func() {
		binds := containerInfo["HostConfig"].(map[string]interface{})["Binds"].([]interface{})
		assert.Contains(suite.T(), binds, tmpDir+":/updated/path")
		assert.NotContains(suite.T(), binds, tmpDir+":/initial/path")
	})

	suite.Run("Updated Network Aliases", func() {
		networkSettings := containerInfo["NetworkSettings"].(map[string]interface{})
		networks := networkSettings["Networks"].(map[string]interface{})
		networkInfo := networks[network].(map[string]interface{})
		aliases := networkInfo["Aliases"].([]interface{})
		assert.Contains(suite.T(), aliases, serviceName)
	})

	suite.Run("Updated Health Checks", func() {
		err = suite.updater.performHealthChecks(serviceName, updatedService.HealthCheck)
		assert.NoError(suite.T(), err)
	})
}
