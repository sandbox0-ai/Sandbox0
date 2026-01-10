package config

import (
	"time"

	"github.com/sandbox0-ai/infra/pkg/env"
)

// Config holds the configuration for the manager
type Config struct {
	// HTTP Server
	HTTPPort int

	// template
	DefaultTemplate          string
	DefaultTemplateNamespace string
	DefaultClusterId         string

	// Kubernetes
	KubeConfig     string
	Namespace      string
	LeaderElection bool
	ResyncPeriod   time.Duration

	// Database
	DatabaseURL string

	// Cleanup Controller
	CleanupInterval time.Duration

	// Logging
	LogLevel string

	// Metrics
	MetricsPort int

	// Webhook
	WebhookPort     int
	WebhookCertPath string
	WebhookKeyPath  string

	// Internal Auth
	InternalAuthPublicKeyPath  string
	InternalAuthPrivateKeyPath string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	return &Config{
		HTTPPort:                   env.GetEnvInt("HTTP_PORT", 8080),
		DefaultTemplate:            env.GetEnv("DEFAULT_TEMPLATE", "default"),
		DefaultTemplateNamespace:   env.GetEnv("DEFAULT_TEMPLATE_NAMESPACE", "sb0"),
		DefaultClusterId:           env.GetEnv("DEFAULT_CLUSTER_ID", "default"),
		KubeConfig:                 env.GetEnv("KUBECONFIG", ""),
		Namespace:                  env.GetEnv("NAMESPACE", "default"),
		LeaderElection:             env.GetEnvBool("LEADER_ELECTION", true),
		ResyncPeriod:               env.GetEnvDuration("RESYNC_PERIOD", 30*time.Second),
		DatabaseURL:                env.GetEnv("DATABASE_URL", ""),
		CleanupInterval:            env.GetEnvDuration("CLEANUP_INTERVAL", 60*time.Second),
		LogLevel:                   env.GetEnv("LOG_LEVEL", "info"),
		MetricsPort:                env.GetEnvInt("METRICS_PORT", 9090),
		WebhookPort:                env.GetEnvInt("WEBHOOK_PORT", 9443),
		WebhookCertPath:            env.GetEnv("WEBHOOK_CERT_PATH", "/tmp/k8s-webhook-server/serving-certs/tls.crt"),
		WebhookKeyPath:             env.GetEnv("WEBHOOK_KEY_PATH", "/tmp/k8s-webhook-server/serving-certs/tls.key"),
		InternalAuthPublicKeyPath:  env.GetEnv("INTERNAL_AUTH_PUBLIC_KEY_PATH", "/config/internal_jwt_public.key"),
		InternalAuthPrivateKeyPath: env.GetEnv("INTERNAL_AUTH_PRIVATE_KEY_PATH", "/secrets/internal_jwt_private.key"),
	}
}
