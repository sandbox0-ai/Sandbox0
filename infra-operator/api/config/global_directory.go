// +kubebuilder:object:generate=true
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GlobalDirectoryConfig holds all configuration for global-directory.
type GlobalDirectoryConfig struct {
	// +optional
	// +kubebuilder:default=8080
	HTTPPort int `yaml:"http_port" json:"httpPort"`
	// +optional
	// +kubebuilder:default="info"
	LogLevel string `yaml:"log_level" json:"logLevel"`

	DatabaseURL string `yaml:"database_url" json:"-"`
	// +optional
	// +kubebuilder:default=30
	DatabaseMaxConns int `yaml:"database_max_conns" json:"databaseMaxConns"`
	// +optional
	// +kubebuilder:default=8
	DatabaseMinConns int `yaml:"database_min_conns" json:"databaseMinConns"`
	// +optional
	// +kubebuilder:default="gd"
	DatabaseSchema string `yaml:"database_schema" json:"databaseSchema"`

	// +optional
	// +kubebuilder:default="5m"
	RegionTokenTTL metav1.Duration `yaml:"region_token_ttl" json:"regionTokenTTL"`

	// License file path used to unlock enterprise SSO features.
	// +optional
	LicenseFile string `yaml:"license_file" json:"-"`

	// +optional
	// +kubebuilder:default="30s"
	ShutdownTimeout metav1.Duration `yaml:"shutdown_timeout" json:"shutdownTimeout"`
	// +optional
	// +kubebuilder:default="30s"
	ServerReadTimeout metav1.Duration `yaml:"server_read_timeout" json:"serverReadTimeout"`
	// +optional
	// +kubebuilder:default="60s"
	ServerWriteTimeout metav1.Duration `yaml:"server_write_timeout" json:"serverWriteTimeout"`
	// +optional
	// +kubebuilder:default="120s"
	ServerIdleTimeout metav1.Duration `yaml:"server_idle_timeout" json:"serverIdleTimeout"`

	// +optional
	GatewayConfig `yaml:",inline" json:",inline"`
}

// LoadGlobalDirectoryConfig returns the global-directory configuration.
func LoadGlobalDirectoryConfig() *GlobalDirectoryConfig {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "/config/config.yaml"
	}

	cfg, err := loadGlobalDirectoryConfig(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config from %s: %v, using empty config\n", path, err)
		cfg = &GlobalDirectoryConfig{}
	}
	return cfg
}

func loadGlobalDirectoryConfig(path string) (*GlobalDirectoryConfig, error) {
	cfg := &GlobalDirectoryConfig{}
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	data = []byte(os.ExpandEnv(string(data)))

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return cfg, nil
}
