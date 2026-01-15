package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for internal-gateway
type Config struct {
	// Server configuration
	HTTPPort int    `yaml:"http_port"`
	LogLevel string `yaml:"log_level"`

	// Upstream services
	ManagerURL      string `yaml:"manager_url"`
	StorageProxyURL string `yaml:"storage_proxy_url"`

	// Internal authentication (for validating requests from edge-gateway and
	// generating tokens for downstream services)
	InternalJWTPrivateKeyPath string `yaml:"internal_jwt_private_key_path"`

	// Timeouts
	ProxyTimeout      time.Duration `yaml:"proxy_timeout"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout"`
	HealthCheckPeriod time.Duration `yaml:"health_check_period"`
}

// defaultConfig returns the default configuration
func defaultConfig() *Config {
	return &Config{
		HTTPPort:                  8443,
		LogLevel:                  "info",
		ManagerURL:                "http://manager.sandbox0-system:8080",
		StorageProxyURL:           "http://storage-proxy.sandbox0-system:8081",
		InternalJWTPrivateKeyPath: "/secrets/internal_jwt_private.key",
		ProxyTimeout:              30 * time.Second,
		ShutdownTimeout:           30 * time.Second,
		HealthCheckPeriod:         10 * time.Second,
	}
}

var Cfg *Config

func init() {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "/config/config.yaml"
	}

	var err error
	Cfg, err = load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config from %s: %v, using defaults\n", path, err)
		Cfg = defaultConfig()
	}
}

// LoadConfig returns the global configuration
func LoadConfig() *Config {
	return Cfg
}

// load loads configuration from a YAML file
func load(path string) (*Config, error) {
	// Default configuration
	cfg := defaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables
	data = []byte(os.ExpandEnv(string(data)))

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}
