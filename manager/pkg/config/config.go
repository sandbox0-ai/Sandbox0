package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the configuration for the manager
type Config struct {
	// HTTP Server
	HTTPPort int `yaml:"http_port"`

	// template
	DefaultTemplate          string `yaml:"default_template"`
	DefaultTemplateNamespace string `yaml:"default_template_namespace"`
	DefaultClusterId         string `yaml:"default_cluster_id"`

	// Kubernetes
	KubeConfig     string        `yaml:"kube_config"`
	Namespace      string        `yaml:"namespace"`
	LeaderElection bool          `yaml:"leader_election"`
	ResyncPeriod   time.Duration `yaml:"resync_period"`

	// Database
	DatabaseURL string `yaml:"database_url"`

	// Cleanup Controller
	CleanupInterval time.Duration `yaml:"cleanup_interval"`

	// Logging
	LogLevel string `yaml:"log_level"`

	// Metrics
	MetricsPort int `yaml:"metrics_port"`

	// Webhook
	WebhookPort     int    `yaml:"webhook_port"`
	WebhookCertPath string `yaml:"webhook_cert_path"`
	WebhookKeyPath  string `yaml:"webhook_key_path"`

	// Internal Auth
	InternalAuthPublicKeyPath  string `yaml:"internal_auth_public_key_path"`
	InternalAuthPrivateKeyPath string `yaml:"internal_auth_private_key_path"`

	// Sandbox
	DefaultSandboxTTL time.Duration `yaml:"default_sandbox_ttl"`
}

// defaultConfig returns the default configuration
func defaultConfig() *Config {
	return &Config{
		HTTPPort:                   8080,
		DefaultTemplate:            "default",
		DefaultTemplateNamespace:   "sb0",
		DefaultClusterId:           "default",
		KubeConfig:                 "",
		Namespace:                  "default",
		LeaderElection:             true,
		ResyncPeriod:               30 * time.Second,
		DatabaseURL:                "",
		CleanupInterval:            60 * time.Second,
		LogLevel:                   "info",
		MetricsPort:                9090,
		WebhookPort:                9443,
		WebhookCertPath:            "/tmp/k8s-webhook-server/serving-certs/tls.crt",
		WebhookKeyPath:             "/tmp/k8s-webhook-server/serving-certs/tls.key",
		InternalAuthPublicKeyPath:  "/config/internal_jwt_public.key",
		InternalAuthPrivateKeyPath: "/secrets/internal_jwt_private.key",
		DefaultSandboxTTL:          5 * time.Minute,
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
