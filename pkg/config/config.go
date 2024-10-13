package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Project  Project   `yaml:"project" validate:"required"`
	Services []Service `yaml:"services" validate:"required,dive"`
	Storages []Storage `yaml:"storages" validate:"required,dive"`
	Volumes  []Volume  `yaml:"volumes" validate:"required,dive"`
}

type Project struct {
	Name   string `yaml:"name" validate:"required"`
	Domain string `yaml:"domain" validate:"required,fqdn"`
	Email  string `yaml:"email" validate:"required,email"`
}

type Service struct {
	Name        string       `yaml:"name" validate:"required"`
	Image       string       `yaml:"image" validate:"required"`
	Port        int          `yaml:"port" validate:"required,min=1,max=65535"`
	HealthCheck *HealthCheck `yaml:"health_check"`
	Routes      []Route      `yaml:"routes" validate:"required,dive"`
	Volumes     []string     `yaml:"volumes" validate:"dive,volume_reference"`

	Forwards []string

	EnvVars []EnvVar
}

type EnvVar struct {
	Name  string
	Value string
}

type HealthCheck struct {
	Path     string
	Interval time.Duration
	Timeout  time.Duration
	Retries  int
}

type Route struct {
	PathPrefix  string `yaml:"path" validate:"required"`
	StripPrefix bool   `yaml:"strip_prefix"`
}

type Storage struct {
	Name    string   `yaml:"name" validate:"required"`
	Image   string   `yaml:"image" validate:"required"`
	Volumes []string `yaml:"volumes" validate:"required,dive,volume_reference"`
}

type Volume struct {
	Name string `yaml:"name" validate:"required"`
	Path string `yaml:"path" validate:"required,unix_path"`
}

func ParseConfig(data []byte) (*Config, error) {
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing YAML: %v", err)
	}

	validate := validator.New()

	_ = validate.RegisterValidation("volume_reference", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		parts := strings.Split(value, ":")
		return len(parts) == 2 && parts[0] != "" && parts[1] != ""
	})

	_ = validate.RegisterValidation("unix_path", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		return strings.HasPrefix(value, "/")
	})

	if err := validate.Struct(config); err != nil {
		return nil, fmt.Errorf("validation error: %v", err)
	}

	return &config, nil
}
