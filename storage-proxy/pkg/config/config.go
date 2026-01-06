package config

import (
	"github.com/sandbox0-ai/infra/pkg/env"
)

// Config holds the server configuration
type Config struct {
	// gRPC Server
	GRPCAddr string
	GRPCPort int

	// HTTP Management API
	HTTPAddr string
	HTTPPort int

	// JuiceFS defaults
	DefaultMetaURL    string
	DefaultS3Region   string
	DefaultS3Endpoint string
	DefaultCacheSize  string
	DefaultCacheDir   string

	// Security
	JWTSecret        string // Deprecated: Use InternalAuthPublicKey instead
	InternalAuthPublicKey string // Ed25519 public key for internal auth (base64 encoded)

	// Cache
	CacheRoot string

	// Monitoring
	MetricsEnabled bool
	MetricsPort    int

	// Logging
	LogLevel  string
	AuditLog  bool
	AuditFile string

	// Rate limiting
	MaxOpsPerSecond   int
	MaxBytesPerSecond int64
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	return &Config{
		GRPCAddr: env.GetEnv("GRPC_ADDR", "0.0.0.0"),
		GRPCPort: env.GetEnvInt("GRPC_PORT", 8080),

		HTTPAddr: env.GetEnv("HTTP_ADDR", "0.0.0.0"),
		HTTPPort: env.GetEnvInt("HTTP_PORT", 8081),

		DefaultMetaURL:    env.GetEnv("DEFAULT_META_URL", ""),
		DefaultS3Region:   env.GetEnv("DEFAULT_S3_REGION", "us-east-1"),
		DefaultS3Endpoint: env.GetEnv("DEFAULT_S3_ENDPOINT", ""),
		DefaultCacheSize:  env.GetEnv("DEFAULT_CACHE_SIZE", "1G"),
		DefaultCacheDir:   env.GetEnv("DEFAULT_CACHE_DIR", "/var/lib/storage-proxy/cache"),

		JWTSecret:             env.GetEnv("JWT_SECRET", ""),
		InternalAuthPublicKey: env.GetEnv("INTERNAL_AUTH_PUBLIC_KEY", ""),

		CacheRoot: env.GetEnv("CACHE_ROOT", "/var/lib/storage-proxy/cache"),

		MetricsEnabled: env.GetEnvBool("METRICS_ENABLED", true),
		MetricsPort:    env.GetEnvInt("METRICS_PORT", 9090),

		LogLevel:  env.GetEnv("LOG_LEVEL", "info"),
		AuditLog:  env.GetEnvBool("AUDIT_LOG", true),
		AuditFile: env.GetEnv("AUDIT_FILE", "/var/log/storage-proxy/audit.log"),

		MaxOpsPerSecond:   env.GetEnvInt("MAX_OPS_PER_SECOND", 10000),
		MaxBytesPerSecond: env.GetEnvInt64("MAX_BYTES_PER_SECOND", 1<<30), // 1GB/s
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Require either new internal auth or legacy JWT secret
	if c.InternalAuthPublicKey == "" && c.JWTSecret == "" {
		return ErrMissingAuthConfig
	}
	return nil
}

// Errors
var (
	ErrMissingJWTSecret  = &ConfigError{"JWT_SECRET is required"}                                        // Deprecated
	ErrMissingAuthConfig = &ConfigError{"INTERNAL_AUTH_PUBLIC_KEY or JWT_SECRET (legacy) is required"}
)

// ConfigError represents a configuration error
type ConfigError struct {
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}
