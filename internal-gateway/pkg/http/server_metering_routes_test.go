package http

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sandbox0-ai/sandbox0/infra-operator/api/config"
	"github.com/sandbox0-ai/sandbox0/internal-gateway/pkg/middleware"
	gatewayhandlers "github.com/sandbox0-ai/sandbox0/pkg/gateway/http/handlers"
	"github.com/sandbox0-ai/sandbox0/pkg/internalauth"
	"go.uber.org/zap"
)

func TestSetupMeteringRoutesMountsInternalEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server, _ := testMeteringRouteServer(t)
	server.setupMeteringRoutes()

	if !hasRoute(server.router, "GET", "/internal/v1/metering/status") {
		t.Fatal("expected metering status route to be mounted")
	}
	if !hasRoute(server.router, "GET", "/internal/v1/metering/events") {
		t.Fatal("expected metering events route to be mounted")
	}
	if !hasRoute(server.router, "GET", "/internal/v1/metering/windows") {
		t.Fatal("expected metering windows route to be mounted")
	}
}

func TestSetupMeteringRoutesDoesNotRequireManagerUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server, generator := testMeteringRouteServer(t)
	server.setupMeteringRoutes()

	token, err := generator.Generate("internal-gateway", "team-1", "user-1", internalauth.GenerateOptions{})
	if err != nil {
		t.Fatalf("generate internal token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/metering/status", nil)
	req.Header.Set(internalauth.DefaultTokenHeader, token)
	rec := httptest.NewRecorder()
	server.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func testMeteringRouteServer(t *testing.T) (*Server, *internalauth.Generator) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	validator := internalauth.NewValidator(internalauth.ValidatorConfig{
		Target:             "internal-gateway",
		PublicKey:          publicKey,
		AllowedCallers:     []string{"edge-gateway"},
		ClockSkewTolerance: 5 * time.Second,
	})
	generator := internalauth.NewGenerator(internalauth.GeneratorConfig{
		Caller:     "edge-gateway",
		PrivateKey: privateKey,
		TTL:        time.Minute,
	})

	server := &Server{
		router:          gin.New(),
		cfg:             &config.InternalGatewayConfig{},
		authMiddleware:  middleware.NewInternalAuthMiddleware(validator, zap.NewNop()),
		logger:          zap.NewNop(),
		meteringHandler: gatewayhandlers.NewMeteringHandler(nil, "aws/us-east-1", zap.NewNop()),
	}

	return server, generator
}

func hasRoute(router *gin.Engine, method, path string) bool {
	for _, route := range router.Routes() {
		if route.Method == method && route.Path == path {
			return true
		}
	}
	return false
}
