package internalgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sandbox0-ai/infra/pkg/auth"
	"github.com/sandbox0-ai/infra/pkg/internalauth"
)

type managerProxyCapture struct {
	lastPath   string
	lastMethod string
	lastTeam   string
	callCount  int
}

func newManagerProxyTestEnv(t *testing.T) (*gatewayTestEnv, *managerProxyCapture) {
	t.Helper()

	keys := gatewayKeyPair{}
	keys.privateKey, keys.publicKey = writeInternalGatewayKeys(t)

	capture := &managerProxyCapture{}
	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.lastPath = r.URL.Path
		capture.lastMethod = r.Method
		capture.lastTeam = r.Header.Get(internalauth.TeamIDHeader)
		capture.callCount++

		token := r.Header.Get(internalauth.DefaultTokenHeader)
		validator := newValidator(t, "manager", keys.publicKey, []string{"internal-gateway"})
		if _, err := validator.Validate(token); err != nil {
			t.Fatalf("validate manager token: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	t.Cleanup(managerServer.Close)

	storageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected storage-proxy request: %s", r.URL.Path)
	}))
	t.Cleanup(storageServer.Close)

	env := newGatewayTestEnv(t, managerServer.URL, storageServer.URL, nil, keys)
	return env, capture
}

func TestInternalGatewayIntegration_ProxyToManager(t *testing.T) {
	keys := gatewayKeyPair{}
	keys.privateKey, keys.publicKey = writeInternalGatewayKeys(t)

	var gotPath string
	var gotTeam string

	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTeam = r.Header.Get(internalauth.TeamIDHeader)

		token := r.Header.Get(internalauth.DefaultTokenHeader)
		validator := newValidator(t, "manager", keys.publicKey, []string{"internal-gateway"})
		claims, err := validator.Validate(token)
		if err != nil {
			t.Fatalf("validate manager token: %v", err)
		}
		if claims.TeamID != "team-1" || claims.UserID != "user-1" {
			t.Fatalf("unexpected claims: %+v", claims)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"templates": []any{},
			"count":     0,
		})
	}))
	t.Cleanup(managerServer.Close)

	storageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected storage-proxy request: %s", r.URL.Path)
	}))
	t.Cleanup(storageServer.Close)

	env := newGatewayTestEnv(t, managerServer.URL, storageServer.URL, nil, keys)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermTemplateRead})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/templates", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if gotPath != "/api/v1/templates" {
		t.Fatalf("expected manager path /api/v1/templates, got %s", gotPath)
	}
	if gotTeam != "team-1" {
		t.Fatalf("expected team header team-1, got %s", gotTeam)
	}
}

func TestInternalGatewayIntegration_TemplateCreate(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermTemplateCreate})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/templates", token, map[string]any{
		"name": "tpl-1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/templates" {
		t.Fatalf("expected manager path /api/v1/templates, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_TemplateGet(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermTemplateRead})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/templates/tpl-1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/templates/tpl-1" {
		t.Fatalf("expected manager path /api/v1/templates/tpl-1, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_TemplateUpdate(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermTemplateWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPut, env.server.URL+"/api/v1/templates/tpl-1", token, map[string]any{
		"name": "tpl-1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/templates/tpl-1" {
		t.Fatalf("expected manager path /api/v1/templates/tpl-1, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPut {
		t.Fatalf("expected method PUT, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_TemplateDelete(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermTemplateDelete})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodDelete, env.server.URL+"/api/v1/templates/tpl-1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/templates/tpl-1" {
		t.Fatalf("expected manager path /api/v1/templates/tpl-1, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodDelete {
		t.Fatalf("expected method DELETE, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_TemplateWarmPool(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermTemplateWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/templates/tpl-1/pool/warm", token, map[string]any{
		"count": 1,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/templates/tpl-1/pool/warm" {
		t.Fatalf("expected manager path /api/v1/templates/tpl-1/pool/warm, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxCreate(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxCreate})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes", token, map[string]any{
		"team_id": "team-1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxes/claim" {
		t.Fatalf("expected manager path /api/v1/sandboxes/claim, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxGet(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxRead})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxes/sbx-1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxes/sbx-1" {
		t.Fatalf("expected manager path /api/v1/sandboxes/sbx-1, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxStatus(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxRead})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxes/sbx-1/status", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxes/sbx-1/status" {
		t.Fatalf("expected manager path /api/v1/sandboxes/sbx-1/status, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxUpdate(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPatch, env.server.URL+"/api/v1/sandboxes/sbx-1", token, map[string]any{
		"ttl": 120,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxes/sbx-1" {
		t.Fatalf("expected manager path /api/v1/sandboxes/sbx-1, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPatch {
		t.Fatalf("expected method PATCH, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxDelete(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxDelete})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodDelete, env.server.URL+"/api/v1/sandboxes/sbx-1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxes/sbx-1" {
		t.Fatalf("expected manager path /api/v1/sandboxes/sbx-1, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodDelete {
		t.Fatalf("expected method DELETE, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxPause(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/pause", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxes/sbx-1/pause" {
		t.Fatalf("expected manager path /api/v1/sandboxes/sbx-1/pause, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxResume(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/resume", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxes/sbx-1/resume" {
		t.Fatalf("expected manager path /api/v1/sandboxes/sbx-1/resume, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxRefresh(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/refresh", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxes/sbx-1/refresh" {
		t.Fatalf("expected manager path /api/v1/sandboxes/sbx-1/refresh, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_NetworkPolicyGet(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxRead})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxes/sbx-1/network", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxes/sbx-1/network" {
		t.Fatalf("expected manager path /api/v1/sandboxes/sbx-1/network, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_NetworkPolicyUpdate(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPatch, env.server.URL+"/api/v1/sandboxes/sbx-1/network", token, map[string]any{
		"mode": "allow_all",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxes/sbx-1/network" {
		t.Fatalf("expected manager path /api/v1/sandboxes/sbx-1/network, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPatch {
		t.Fatalf("expected method PATCH, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_BandwidthPolicyGet(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxRead})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxes/sbx-1/bandwidth", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxes/sbx-1/bandwidth" {
		t.Fatalf("expected manager path /api/v1/sandboxes/sbx-1/bandwidth, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_BandwidthPolicyUpdate(t *testing.T) {
	env, capture := newManagerProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPatch, env.server.URL+"/api/v1/sandboxes/sbx-1/bandwidth", token, map[string]any{
		"egress_rate_bps": 1,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxes/sbx-1/bandwidth" {
		t.Fatalf("expected manager path /api/v1/sandboxes/sbx-1/bandwidth, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPatch {
		t.Fatalf("expected method PATCH, got %s", capture.lastMethod)
	}
}
