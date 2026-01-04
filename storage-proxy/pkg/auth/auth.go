package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TokenClaims represents JWT token claims
type TokenClaims struct {
	VolumeID  string `json:"volume_id"`
	SandboxID string `json:"sandbox_id"`
	TeamID    string `json:"team_id"`
	jwt.RegisteredClaims
}

// Authenticator handles JWT token validation
type Authenticator struct {
	jwtSecret []byte
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(jwtSecret string) *Authenticator {
	return &Authenticator{
		jwtSecret: []byte(jwtSecret),
	}
}

// UnaryInterceptor returns a gRPC unary interceptor for authentication
func (a *Authenticator) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Skip authentication for volume management endpoints (they use manager tokens)
		if strings.HasSuffix(info.FullMethod, "MountVolume") ||
			strings.HasSuffix(info.FullMethod, "UnmountVolume") {
			return handler(ctx, req)
		}

		// Extract and validate token
		claims, err := a.authenticate(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}

		// Add claims to context
		ctx = context.WithValue(ctx, claimsKey, claims)

		return handler(ctx, req)
	}
}

// authenticate validates JWT token and returns claims
func (a *Authenticator) authenticate(ctx context.Context) (*TokenClaims, error) {
	// Extract token from metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("missing metadata")
	}

	authHeaders := md["authorization"]
	if len(authHeaders) == 0 {
		return nil, fmt.Errorf("missing authorization header")
	}

	auth := authHeaders[0]
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, fmt.Errorf("invalid authorization format")
	}

	tokenString := strings.TrimPrefix(auth, "Bearer ")

	// Parse and validate token
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return a.jwtSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	// Validate expiration
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("token expired")
	}

	return claims, nil
}

// GetClaims extracts token claims from context
func GetClaims(ctx context.Context) (*TokenClaims, error) {
	claims, ok := ctx.Value(claimsKey).(*TokenClaims)
	if !ok {
		return nil, fmt.Errorf("claims not found in context")
	}
	return claims, nil
}

// GenerateToken generates a JWT token for storage access
func (a *Authenticator) GenerateToken(volumeID, sandboxID, teamID string, expiresIn time.Duration) (string, error) {
	now := time.Now()
	claims := &TokenClaims{
		VolumeID:  volumeID,
		SandboxID: sandboxID,
		TeamID:    teamID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(expiresIn)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "storage-proxy",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.jwtSecret)
}

// Context key for claims
type contextKey string

const claimsKey contextKey = "claims"
