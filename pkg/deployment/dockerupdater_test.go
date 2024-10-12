package deployment

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type MockExecutor struct {
	mock.Mock
}

func (m *MockExecutor) RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error) {
	args = append([]string{command}, args...)
	called := m.Called(ctx, args)
	return strings.NewReader(called.String(0)), called.Error(1)
}

type DockerUpdaterTestSuite struct {
	suite.Suite
	updater *DockerUpdater
	mock    *MockExecutor
}

func (suite *DockerUpdaterTestSuite) SetupTest() {
	suite.mock = new(MockExecutor)
	suite.updater = NewDockerUpdater(suite.mock)
}

func (suite *DockerUpdaterTestSuite) TestGetImageName_Success() {
	service := "test-service"
	network := "test-network"
	expectedImage := "test-image:latest"

	suite.mock.On("RunCommand", mock.Anything, []string{"docker", "ps", "-q", "--filter", "network=test-network"}).Return("container1", nil)
	suite.mock.On("RunCommand", mock.Anything, []string{"docker", "inspect", "container1"}).Return(`[
		{
			"Id": "container1",
			"Config": {
				"Image": "test-image:latest"
			},
			"NetworkSettings": {
				"Networks": {
					"test-network": {
						"Aliases": ["test-service"]
					}
				}
			}
		}
	]`, nil)

	imageName, err := suite.updater.getImageName(service, network)

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), expectedImage, imageName)
	suite.mock.AssertExpectations(suite.T())
}

func (suite *DockerUpdaterTestSuite) TestGetImageName_NoContainerFound() {
	service := "test-service"
	network := "test-network"

	suite.mock.On("RunCommand", mock.Anything, []string{"docker", "ps", "-q", "--filter", "network=test-network"}).Return("", nil)

	imageName, err := suite.updater.getImageName(service, network)

	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "no container found with alias test-service in network test-network")
	assert.Empty(suite.T(), imageName)
	suite.mock.AssertExpectations(suite.T())
}

func (suite *DockerUpdaterTestSuite) TestGetImageName_DockerPsError() {
	service := "test-service"
	network := "test-network"

	suite.mock.On("RunCommand", mock.Anything, []string{"docker", "ps", "-q", "--filter", "network=test-network"}).Return("", assert.AnError)

	imageName, err := suite.updater.getImageName(service, network)

	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "failed to get container IDs")
	assert.Empty(suite.T(), imageName)
	suite.mock.AssertExpectations(suite.T())
}

func TestDockerUpdaterSuite(t *testing.T) {
	suite.Run(t, new(DockerUpdaterTestSuite))
}
