package internalgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sandbox0-ai/infra/pkg/internalauth"
)

func TestInternalGatewayIntegration_InternalClusterSummary(t *testing.T) {
	keys := gatewayKeyPair{}
	keys.privateKey, keys.publicKey = writeInternalGatewayKeys(t)

	schedulerPerms := []string{"sandbox:read"}
	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/cluster/summary" {
			t.Fatalf("unexpected manager path: %s", r.URL.Path)
		}

		token := r.Header.Get(internalauth.DefaultTokenHeader)
		validator := newValidator(t, "manager", keys.publicKey, []string{"internal-gateway"})
		claims, err := validator.Validate(token)
		if err != nil {
			t.Fatalf("validate manager token: %v", err)
		}
		if len(claims.Permissions) != 1 || claims.Permissions[0] != "sandbox:read" {
			t.Fatalf("unexpected permissions: %v", claims.Permissions)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	t.Cleanup(managerServer.Close)

	storageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected storage-proxy request: %s", r.URL.Path)
	}))
	t.Cleanup(storageServer.Close)

	env := newGatewayTestEnv(t, managerServer.URL, storageServer.URL, schedulerPerms, keys)
	token := newInternalToken(t, env.schedulerGen, []string{"*:*"})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/internal/v1/cluster/summary", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestInternalGatewayIntegration_InternalTemplateStats(t *testing.T) {
	keys := gatewayKeyPair{}
	keys.privateKey, keys.publicKey = writeInternalGatewayKeys(t)

	schedulerPerms := []string{"template:read"}
	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/templates/stats" {
			t.Fatalf("unexpected manager path: %s", r.URL.Path)
		}

		token := r.Header.Get(internalauth.DefaultTokenHeader)
		validator := newValidator(t, "manager", keys.publicKey, []string{"internal-gateway"})
		claims, err := validator.Validate(token)
		if err != nil {
			t.Fatalf("validate manager token: %v", err)
		}
		if len(claims.Permissions) != 1 || claims.Permissions[0] != "template:read" {
			t.Fatalf("unexpected permissions: %v", claims.Permissions)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	t.Cleanup(managerServer.Close)

	storageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected storage-proxy request: %s", r.URL.Path)
	}))
	t.Cleanup(storageServer.Close)

	env := newGatewayTestEnv(t, managerServer.URL, storageServer.URL, schedulerPerms, keys)
	token := newInternalToken(t, env.schedulerGen, []string{"*:*"})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/internal/v1/templates/stats", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
}
