package proxy

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/yarlson/aerie/config"
)

// GenerateNginxConfig generates an Nginx configuration based on the provided config.
func GenerateNginxConfig(cfg *config.Config) (string, error) {
	if cfg.Project.Domain == "" {
		cfg.Project.Domain = "localhost"
	}

	tmpl := template.Must(template.New("nginx").Parse(`
http {
{{- range .Services}}
	upstream {{.Name}} {
		server {{.Name}}:{{.Port}};
	}
{{- end}}

	server {
		listen 80;
		server_name {{.Project.Domain}};
		return 301 https://$server_name$request_uri;
	}

	server {
		listen 443 ssl http2;
		server_name {{.Project.Domain}};

		ssl_certificate /etc/certificates/{{.Project.Domain}}/fullchain.pem;
		ssl_certificate_key /etc/certificates/{{.Project.Domain}}/privkey.pem;
		ssl_protocols TLSv1.2 TLSv1.3;
		ssl_prefer_server_ciphers on;

{{- range .Services}}
	{{- $serviceName := .Name }}
	{{- range .Routes}}
		location {{.PathPrefix}} {
		{{- if .StripPrefix}}
			rewrite ^{{.PathPrefix}}(.*)$ /$1 break;
		{{- end}}
			proxy_pass http://{{$serviceName}};
		}
	{{- end}}
{{- end}}
	}
}
`))

	var buffer bytes.Buffer
	err := tmpl.Execute(&buffer, cfg)
	if err != nil {
		return "", err
	}

	return strings.ReplaceAll(buffer.String(), "\t", "    "), nil
}
