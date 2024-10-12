package proxy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/yarlson/aerie/config"
)

type ProxyTestSuite struct {
	suite.Suite
}

func TestProxySuite(t *testing.T) {
	suite.Run(t, new(ProxyTestSuite))
}

func stripWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return strings.Join(result, "\n")
}

func (suite *ProxyTestSuite) TestGenerateNginxConfig_Success() {
	cfg := &config.Config{
		Project: config.Project{
			Name:   "test-project",
			Domain: "test.example.com",
			Email:  "test@example.com",
		},
		Services: []config.Service{
			{
				Name:  "web",
				Image: "nginx:latest",
				Port:  80,
				Routes: []config.Route{
					{
						PathPrefix:  "/",
						StripPrefix: false,
					},
				},
			},
		},
	}

	expectedConfig := `
http {
    upstream web {
        server web:80;
    }

    server {
        listen 80;
        server_name test.example.com;
        return 301 https://$server_name$request_uri;
    }

    server {
        listen 443 ssl http2;
        server_name test.example.com;

        ssl_certificate /etc/certificates/test.example.com/fullchain.pem;
        ssl_certificate_key /etc/certificates/test.example.com/privkey.pem;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_prefer_server_ciphers on;

        location / {
            proxy_pass http://web;
        }
    }
}
`

	nginxConfig, err := GenerateNginxConfig(cfg)

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), stripWhitespace(expectedConfig), stripWhitespace(nginxConfig))
}

func (suite *ProxyTestSuite) TestGenerateNginxConfig_MultipleServices() {
	cfg := &config.Config{
		Project: config.Project{
			Name:   "test-project",
			Domain: "test.example.com",
			Email:  "test@example.com",
		},
		Services: []config.Service{
			{
				Name:  "web",
				Image: "nginx:latest",
				Port:  80,
				Routes: []config.Route{
					{
						PathPrefix:  "/",
						StripPrefix: false,
					},
				},
			},
			{
				Name:  "api",
				Image: "api:latest",
				Port:  8080,
				Routes: []config.Route{
					{
						PathPrefix:  "/api",
						StripPrefix: true,
					},
				},
			},
		},
	}

	expectedConfig := `
http {
    upstream web {
        server web:80;
    }

    upstream api {
        server api:8080;
    }

    server {
        listen 80;
        server_name test.example.com;
        return 301 https://$server_name$request_uri;
    }

    server {
        listen 443 ssl http2;
        server_name test.example.com;

        ssl_certificate /etc/certificates/test.example.com/fullchain.pem;
        ssl_certificate_key /etc/certificates/test.example.com/privkey.pem;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_prefer_server_ciphers on;

        location / {
            proxy_pass http://web;
        }

        location /api {
            rewrite ^/api(.*)$ /$1 break;
            proxy_pass http://api;
        }
    }
}
`

	nginxConfig, err := GenerateNginxConfig(cfg)

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), stripWhitespace(expectedConfig), stripWhitespace(nginxConfig))
}

func (suite *ProxyTestSuite) TestGenerateNginxConfig_EmptyConfig() {
	cfg := &config.Config{}

	expectedConfig := `
http {
    server {
        listen 80;
        server_name localhost;
        return 301 https://$server_name$request_uri;
    }

    server {
        listen 443 ssl http2;
        server_name localhost;

        ssl_certificate /etc/certificates/localhost/fullchain.pem;
        ssl_certificate_key /etc/certificates/localhost/privkey.pem;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_prefer_server_ciphers on;
    }
}
`

	nginxConfig, err := GenerateNginxConfig(cfg)

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), stripWhitespace(expectedConfig), stripWhitespace(nginxConfig))
}
