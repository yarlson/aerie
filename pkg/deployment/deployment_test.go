package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/yarlson/ftl/pkg/config"
)

type DeploymentTestSuite struct {
	suite.Suite
	updater *Deployment
	network string
}

func TestDeploymentSuite(t *testing.T) {
	suite.Run(t, new(DeploymentTestSuite))
}

type LocalExecutor struct{}

func (e *LocalExecutor) RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	var combinedOutput bytes.Buffer
	cmd.Stdout = &combinedOutput
	cmd.Stderr = &combinedOutput

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return bytes.NewReader(combinedOutput.Bytes()), nil
}

func (e *LocalExecutor) CopyFile(ctx context.Context, src, dst string) error {
	cmd := exec.CommandContext(ctx, "cp", src, dst)

	return cmd.Run()
}

func (suite *DeploymentTestSuite) SetupSuite() {
	suite.network = "ftl-test-network"
	_ = exec.Command("docker", "network", "create", suite.network).Run()
}

func (suite *DeploymentTestSuite) TearDownSuite() {
	_ = exec.Command("docker", "network", "rm", suite.network).Run()
}

func (suite *DeploymentTestSuite) SetupTest() {
	executor := &LocalExecutor{}
	suite.updater = NewDeployment(executor)
}

func (suite *DeploymentTestSuite) createTempDir() string {
	tmpDir, err := os.MkdirTemp("", "docker-test")
	assert.NoError(suite.T(), err)
	return tmpDir
}

func (suite *DeploymentTestSuite) removeContainer(containerName string) {
	_ = exec.Command("docker", "stop", containerName).Run() // nolint: errcheck
	_ = exec.Command("docker", "rm", "-f", containerName).Run()
}

func (suite *DeploymentTestSuite) removeVolume(volumeName string) {
	_ = exec.Command("docker", "volume", "rm", volumeName).Run() // nolint: errcheck
}

func (suite *DeploymentTestSuite) inspectContainer(containerName string) map[string]interface{} {
	cmd := exec.Command("docker", "inspect", containerName)
	output, err := cmd.Output()
	assert.NoError(suite.T(), err)

	var containerInfo []map[string]interface{}
	err = json.Unmarshal(output, &containerInfo)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), containerInfo, 1)

	return containerInfo[0]
}

func (suite *DeploymentTestSuite) TestUpdateService() {
	tmpDir := suite.createTempDir()
	defer os.RemoveAll(tmpDir)

	const (
		project     = "test-project"
		serviceName = "test-update-service"
		network     = "ftl-test-network"
		proxyName   = "nginx-proxy"
		proxyPort   = "443"
	)

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
	suite.removeContainer(proxyName)

	err := suite.updater.InstallService(initialService, network)
	assert.NoError(suite.T(), err)

	defer suite.removeContainer(serviceName)
	defer suite.removeContainer(proxyName)

	cfg := &config.Config{
		Project: config.Project{
			Name:   project,
			Domain: "localhost",
			Email:  "test@example.com",
		},
		Services: []config.Service{
			{
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
				Routes: []config.Route{
					{
						PathPrefix:  "/",
						StripPrefix: false,
					},
				},
			},
		},
		Storages: []config.Storage{
			{
				Name:  "storage",
				Image: "postgres:16",
				Volumes: []string{
					"postgres_data:/var/lib/postgresql/data",
				},
			},
		},
		Volumes: []string{
			"postgres_data",
		},
	}

	projectPath, err := suite.updater.prepareProjectFolder(project)
	assert.NoError(suite.T(), err)

	proxyCertPath := filepath.Join(projectPath, "localhost.crt")
	proxyKeyPath := filepath.Join(projectPath, "localhost.key")
	mkcertCmds := [][]string{
		{"mkcert", "-install"},
		{"mkcert", "-cert-file", proxyCertPath, "-key-file", proxyKeyPath, "localhost"},
	}

	for _, cmd := range mkcertCmds {
		if output, err := suite.updater.runCommand(context.Background(), cmd[0], cmd[1:]...); err != nil {
			assert.NoError(suite.T(), err)
			suite.T().Log(output)
			return
		}
	}

	suite.removeContainer("proxy")
	suite.removeContainer("storage")
	suite.removeVolume("postgres_data")

	err = suite.updater.StartProxy(project, cfg, network)
	assert.NoError(suite.T(), err)

	defer func() {
		suite.removeContainer("proxy")
		suite.removeContainer("storage")
		suite.removeVolume("postgres_data")
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requestStats := struct {
		totalRequests  int32
		failedRequests int32
	}{}

	time.Sleep(5 * time.Second)

	for i := 0; i < 10; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					resp, err := http.Get("https://localhost:" + proxyPort)
					atomic.AddInt32(&requestStats.totalRequests, 1)
					if err != nil || resp.StatusCode != http.StatusOK {
						atomic.AddInt32(&requestStats.failedRequests, 1)
					}
					if resp != nil {
						_ = resp.Body.Close()
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
	}

	time.Sleep(2 * time.Second)

	initialContainerID, err := suite.updater.getContainerID(serviceName, network)
	assert.NoError(suite.T(), err)

	updatedService := &cfg.Services[0]

	err = suite.updater.UpdateService(updatedService, network)
	assert.NoError(suite.T(), err)

	updatedContainerID, err := suite.updater.getContainerID(serviceName, network)
	assert.NoError(suite.T(), err)

	assert.NotEqual(suite.T(), initialContainerID, updatedContainerID)

	time.Sleep(2 * time.Second)

	cancel()

	fmt.Printf("Total requests: %d\n", requestStats.totalRequests)
	fmt.Printf("Failed requests: %d\n", requestStats.failedRequests)

	assert.Equal(suite.T(), int32(0), requestStats.failedRequests, "Expected zero failed requests during zero-downtime deployment")

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

func (suite *DeploymentTestSuite) TestMakeProjectFolder() {
	projectName := "test-project"

	suite.Run("Successful folder creation", func() {
		err := suite.updater.makeProjectFolder(projectName)
		suite.Require().NoError(err)
	})
}

func (suite *DeploymentTestSuite) TestDeploy() {
	projectName := "test-project"
	network := "ftl-test-network"

	cfg := &config.Config{
		Project: config.Project{
			Name:   projectName,
			Domain: "localhost",
			Email:  "test@example.com",
		},
		Services: []config.Service{
			{
				Name:  "web",
				Image: "nginx:1.20",
				Port:  80,
				Routes: []config.Route{
					{
						PathPrefix:  "/",
						StripPrefix: false,
					},
				},
			},
		},
		Storages: []config.Storage{
			{
				Name:  "postgres",
				Image: "postgres:16",
				Volumes: []string{
					"postgres_data:/var/lib/postgresql/data",
				},
				EnvVars: []config.EnvVar{
					{Name: "POSTGRES_PASSWORD", Value: "S3cret"},
					{Name: "POSTGRES_USER", Value: "test"},
					{Name: "POSTGRES_DB", Value: "test"},
				},
			},
			{
				Name:  "mysql",
				Image: "mysql:8",
				Volumes: []string{
					"mysql_data:/var/lib/mysql",
				},
				EnvVars: []config.EnvVar{
					{Name: "MYSQL_ROOT_PASSWORD", Value: "S3cret"},
					{Name: "MYSQL_DATABASE", Value: "test"},
					{Name: "MYSQL_USER", Value: "test"},
					{Name: "MYSQL_PASSWORD", Value: "S3cret"},
				},
			},
			{
				Name:  "mongodb",
				Image: "mongo:latest",
				Volumes: []string{
					"mongodb_data:/data/db",
				},
				EnvVars: []config.EnvVar{
					{Name: "MONGO_INITDB_ROOT_USERNAME", Value: "root"},
					{Name: "MONGO_INITDB_ROOT_PASSWORD", Value: "S3cret"},
				},
			},
			{
				Name:  "redis",
				Image: "redis:latest",
				Volumes: []string{
					"redis_data:/data",
				},
			},
			{
				Name:  "rabbitmq",
				Image: "rabbitmq:management",
				Volumes: []string{
					"rabbitmq_data:/var/lib/rabbitmq",
				},
				EnvVars: []config.EnvVar{
					{Name: "RABBITMQ_DEFAULT_USER", Value: "user"},
					{Name: "RABBITMQ_DEFAULT_PASS", Value: "S3cret"},
				},
			},
			{
				Name:  "elasticsearch",
				Image: "elasticsearch:7.14.0",
				Volumes: []string{
					"elasticsearch_data:/usr/share/elasticsearch/data",
				},
				EnvVars: []config.EnvVar{
					{Name: "discovery.type", Value: "single-node"},
					{Name: "ES_JAVA_OPTS", Value: "-Xms512m -Xmx512m"},
				},
			},
		},
		Volumes: []string{
			"postgres_data",
			"mysql_data",
			"mongodb_data",
			"redis_data",
			"rabbitmq_data",
			"elasticsearch_data",
		},
	}

	projectPath, err := suite.updater.prepareProjectFolder(projectName)
	assert.NoError(suite.T(), err)

	proxyCertPath := filepath.Join(projectPath, "localhost.crt")
	proxyKeyPath := filepath.Join(projectPath, "localhost.key")
	mkcertCmds := [][]string{
		{"mkcert", "-install"},
		{"mkcert", "-cert-file", proxyCertPath, "-key-file", proxyKeyPath, "localhost"},
	}

	for _, cmd := range mkcertCmds {
		if output, err := suite.updater.runCommand(context.Background(), cmd[0], cmd[1:]...); err != nil {
			assert.NoError(suite.T(), err)
			suite.T().Log(output)
			return
		}
	}

	suite.removeContainer("proxy")
	suite.removeContainer("web")
	suite.removeContainer("postgres")
	suite.removeContainer("mysql")
	suite.removeContainer("mongodb")
	suite.removeContainer("redis")
	suite.removeContainer("rabbitmq")
	suite.removeContainer("elasticsearch")
	suite.removeVolume("test-project-postgres_data")
	suite.removeVolume("test-project-mysql_data")
	suite.removeVolume("test-project-mongodb_data")
	suite.removeVolume("test-project-redis_data")
	suite.removeVolume("test-project-rabbitmq_data")
	suite.removeVolume("test-project-elasticsearch_data")

	defer func() {
		suite.removeContainer("proxy")
		suite.removeContainer("web")
		suite.removeContainer("postgres")
		suite.removeContainer("mysql")
		suite.removeContainer("mongodb")
		suite.removeContainer("redis")
		suite.removeContainer("rabbitmq")
		suite.removeContainer("elasticsearch")
		suite.removeVolume("test-project-postgres_data")
		suite.removeVolume("test-project-mysql_data")
		suite.removeVolume("test-project-mongodb_data")
		suite.removeVolume("test-project-redis_data")
		suite.removeVolume("test-project-rabbitmq_data")
		suite.removeVolume("test-project-elasticsearch_data")
	}()

	suite.Run("Successful deployment", func() {
		err = suite.updater.Deploy(network, cfg)
		assert.NoError(suite.T(), err)
	})
}
