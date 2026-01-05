package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for internal-gateway
type Config struct {
	// Server configuration
	HTTPPort int
	LogLevel string

	// Database configuration
	DatabaseURL string

	// Upstream services
	ManagerURL      string
	StorageProxyURL string

	// Authentication
	JWTSecret string

	// Rate limiting
	RateLimitRPS   int
	RateLimitBurst int

	// Timeouts
	ProxyTimeout      time.Duration
	ShutdownTimeout   time.Duration
	HealthCheckPeriod time.Duration

	// Feature flags
	EnableMetrics bool
	EnableAudit   bool
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		// Server
		HTTPPort: getEnvInt("GATEWAY_HTTP_PORT", 8443),
		LogLevel: getEnv("GATEWAY_LOG_LEVEL", "info"),

		// Database
		DatabaseURL: getEnv("DATABASE_URL", "postgres://localhost:5432/sandbox0?sslmode=disable"),

		// Upstream services
		ManagerURL:      getEnv("MANAGER_URL", "http://manager:8080"),
		StorageProxyURL: getEnv("STORAGE_PROXY_URL", "http://storage-proxy:8081"),

		// Authentication
		JWTSecret: getEnv("JWT_SECRET", ""),

		// Rate limiting (per team)
		RateLimitRPS:   getEnvInt("RATE_LIMIT_RPS", 100),
		RateLimitBurst: getEnvInt("RATE_LIMIT_BURST", 200),

		// Timeouts
		ProxyTimeout:      getEnvDuration("PROXY_TIMEOUT", 30*time.Second),
		ShutdownTimeout:   getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		HealthCheckPeriod: getEnvDuration("HEALTH_CHECK_PERIOD", 10*time.Second),

		// Features
		EnableMetrics: getEnvBool("ENABLE_METRICS", true),
		EnableAudit:   getEnvBool("ENABLE_AUDIT", true),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

