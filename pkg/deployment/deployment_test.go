package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func (suite *DeploymentTestSuite) TestDeploy() {
	project := "test-project"

	cfg := &config.Config{
		Project: config.Project{
			Name:   project,
			Domain: "localhost",
			Email:  "test@example.com",
		},
		Services: []config.Service{
			{
				Name:  "web",
				Image: "nginx:1.19",
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
				EnvVars: map[string]string{
					"POSTGRES_PASSWORD": "S3cret",
					"POSTGRES_USER":     "test",
					"POSTGRES_DB":       "test",
				},
			},
			{
				Name:  "mysql",
				Image: "mysql:8",
				Volumes: []string{
					"mysql_data:/var/lib/mysql",
				},
				EnvVars: map[string]string{
					"MYSQL_ROOT_PASSWORD": "S3cret",
					"MYSQL_DATABASE":      "test",
					"MYSQL_USER":          "test",
					"MYSQL_PASSWORD":      "S3cret",
				},
			},
			{
				Name:  "mongodb",
				Image: "mongo:latest",
				Volumes: []string{
					"mongodb_data:/data/db",
				},
				EnvVars: map[string]string{
					"MONGO_INITDB_ROOT_USERNAME": "root",
					"MONGO_INITDB_ROOT_PASSWORD": "S3cret",
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
				EnvVars: map[string]string{
					"RABBITMQ_DEFAULT_USER": "user",
					"RABBITMQ_DEFAULT_PASS": "S3cret",
				},
			},
			{
				Name:  "elasticsearch",
				Image: "elasticsearch:7.14.0",
				Volumes: []string{
					"elasticsearch_data:/usr/share/elasticsearch/data",
				},
				EnvVars: map[string]string{
					"discovery.type": "single-node",
					"ES_JAVA_OPTS":   "-Xms512m -Xmx512m",
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
		err = suite.updater.Deploy(project, cfg)
		time.Sleep(5 * time.Second)

		requestStats := struct {
			totalRequests  int32
			failedRequests int32
		}{}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		for i := 0; i < 10; i++ {
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					default:
						resp, err := http.Get("https://localhost/")
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
		assert.NoError(suite.T(), err)

		cfg.Services[0].Image = "nginx:1.20"

		err = suite.updater.Deploy(project, cfg)
		assert.NoError(suite.T(), err)

		fmt.Printf("Total requests: %d\n", requestStats.totalRequests)
		fmt.Printf("Failed requests: %d\n", requestStats.failedRequests)

		serviceName := "web"

		assert.Equal(suite.T(), int32(0), requestStats.failedRequests, "Expected zero failed requests during zero-downtime deployment")
		containerInfo := suite.inspectContainer(serviceName)

		suite.Run("Updated Container State and Config", func() {
			assert.Equal(suite.T(), "running", containerInfo["State"].(map[string]interface{})["Status"])
			assert.Contains(suite.T(), containerInfo["Config"].(map[string]interface{})["Image"], "nginx:1.20")
			assert.Equal(suite.T(), project, containerInfo["HostConfig"].(map[string]interface{})["NetworkMode"])
		})

		suite.Run("Updated Network Aliases", func() {
			networkSettings := containerInfo["NetworkSettings"].(map[string]interface{})
			networks := networkSettings["Networks"].(map[string]interface{})
			networkInfo := networks[project].(map[string]interface{})
			aliases := networkInfo["Aliases"].([]interface{})
			assert.Contains(suite.T(), aliases, serviceName)
		})
	})
}
