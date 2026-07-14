package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Endpoint          string        `yaml:"endpoint"`
	ServerName        string        `yaml:"server_name"`
	Environment       string        `yaml:"environment"`
	EnrollmentFile    string        `yaml:"enrollment_file"`
	CredentialFile    string        `yaml:"credential_file"`
	AllowInsecureHTTP bool          `yaml:"allow_insecure_http"`
	RequestTimeout    time.Duration `yaml:"-"`
}

func Load(path string) (Config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.EnrollmentFile == "" {
		cfg.EnrollmentFile = "/etc/noxwatch/enrollment-token"
	}
	if cfg.CredentialFile == "" {
		cfg.CredentialFile = "/etc/noxwatch/credential.json"
	}
	cfg.RequestTimeout = 10 * time.Second
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	endpoint, err := url.Parse(c.Endpoint)
	if err != nil || endpoint.Host == "" {
		return errors.New("endpoint must be an absolute URL")
	}
	if endpoint.Scheme != "https" && !(c.AllowInsecureHTTP && endpoint.Scheme == "http") {
		return errors.New("endpoint must use HTTPS unless allow_insecure_http is enabled")
	}
	if c.ServerName == "" || len(c.ServerName) > 100 {
		return errors.New("server_name is required and must not exceed 100 characters")
	}
	switch c.Environment {
	case "production", "staging", "development", "testing", "other":
	default:
		return errors.New("environment is invalid")
	}
	return nil
}
