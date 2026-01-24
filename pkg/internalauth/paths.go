package internalauth

import "os"

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

var (
	// DefaultInternalJWTPublicKeyPath is the default path inside containers where the
	// internal auth public key is mounted.
	DefaultInternalJWTPublicKeyPath = getEnv("INTERNAL_JWT_PUBLIC_KEY_PATH", "/config/internal_jwt_public.key")

	// DefaultInternalJWTPrivateKeyPath is the default path inside containers where the
	// internal auth private key is mounted.
	DefaultInternalJWTPrivateKeyPath = getEnv("INTERNAL_JWT_PRIVATE_KEY_PATH", "/secrets/internal_jwt_private.key")
)
