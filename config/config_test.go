package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ConfigTestSuite struct {
	suite.Suite
}

func TestConfigSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}

func (suite *ConfigTestSuite) TestParseConfig_Success() {
	yamlPath := filepath.Join("sample", "aerie.yaml")
	yamlData, err := os.ReadFile(yamlPath)
	assert.NoError(suite.T(), err)

	config, err := ParseConfig(yamlData)

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), config)
	assert.Equal(suite.T(), "my-project", config.Project.Name)
	assert.Equal(suite.T(), "my-project.example.com", config.Project.Domain)
	assert.Equal(suite.T(), "my-project@example.com", config.Project.Email)
	assert.Len(suite.T(), config.Services, 1)
	assert.Len(suite.T(), config.Storages, 1)
	assert.Len(suite.T(), config.Volumes, 1)
}

func (suite *ConfigTestSuite) TestParseConfig_InvalidYAML() {
	yamlData := []byte(`
project:
  name: "test-project"
  domain: "example.com"
  email: "test@example.com"
services:
  - name: "web"
    image: "nginx:latest"
    path: "/app/web"
  - this is invalid YAML
`)

	config, err := ParseConfig(yamlData)

	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), config)
	assert.Contains(suite.T(), err.Error(), "error parsing YAML")
}

func (suite *ConfigTestSuite) TestParseConfig_MissingRequiredFields() {
	yamlData := []byte(`
project:
  name: "test-project"
  domain: "example.com"
services:
  - name: "web"
    image: "nginx:latest"
    path: "/app/web"
storages:
  - name: "db"
    image: "postgres:13"
volumes:
  - name: "db_data"
    path: "/data/db"
`)

	config, err := ParseConfig(yamlData)

	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), config)
	assert.Contains(suite.T(), err.Error(), "validation error")
	assert.Contains(suite.T(), err.Error(), "Project.Email")
}

func (suite *ConfigTestSuite) TestParseConfig_InvalidEmail() {
	yamlData := []byte(`
project:
  name: "test-project"
  domain: "example.com"
  email: "invalid-email"
services:
  - name: "web"
    image: "nginx:latest"
    path: "/app/web"
storages:
  - name: "db"
    image: "postgres:13"
    volumes:
      - "db_data:/var/lib/postgresql/data"
volumes:
  - name: "db_data"
    path: "/data/db"
`)

	config, err := ParseConfig(yamlData)

	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), config)
	assert.Contains(suite.T(), err.Error(), "validation error")
	assert.Contains(suite.T(), err.Error(), "Project.Email")
}

func (suite *ConfigTestSuite) TestParseConfig_InvalidVolumePath() {
	yamlData := []byte(`
project:
  name: "test-project"
  domain: "example.com"
  email: "test@example.com"
services:
  - name: "web"
    image: "nginx:latest"
    path: "/app/web"
storages:
  - name: "db"
    image: "postgres:13"
    volumes:
      - "db_data:/var/lib/postgresql/data"
volumes:
  - name: "db_data"
    path: "invalid_path"
`)

	config, err := ParseConfig(yamlData)

	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), config)
	assert.Contains(suite.T(), err.Error(), "validation error")
	assert.Contains(suite.T(), err.Error(), "Config.Volumes[0].Path")
}

func (suite *ConfigTestSuite) TestParseConfig_InvalidVolumeReference() {
	yamlData := []byte(`
project:
  name: "test-project"
  domain: "example.com"
  email: "test@example.com"
services:
  - name: "web"
    image: "nginx:latest"
    path: "/app/web"
storages:
  - name: "db"
    image: "postgres:13"
    volumes:
      - "invalid_volume_reference"
volumes:
  - name: "db_data"
    path: "/data/db"
`)

	config, err := ParseConfig(yamlData)

	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), config)
	assert.Contains(suite.T(), err.Error(), "validation error")
	assert.Contains(suite.T(), err.Error(), "Config.Storages[0].Volumes[0]")
}
