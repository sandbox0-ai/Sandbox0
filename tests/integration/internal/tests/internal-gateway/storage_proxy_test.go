package internalgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sandbox0-ai/infra/pkg/auth"
	"github.com/sandbox0-ai/infra/pkg/internalauth"
)

type storageProxyCapture struct {
	lastPath   string
	lastMethod string
	lastTeam   string
	callCount  int
}

func newStorageProxyTestEnv(t *testing.T) (*gatewayTestEnv, *storageProxyCapture) {
	t.Helper()

	keys := gatewayKeyPair{}
	keys.privateKey, keys.publicKey = writeInternalGatewayKeys(t)

	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected manager request: %s", r.URL.Path)
	}))
	t.Cleanup(managerServer.Close)

	capture := &storageProxyCapture{}
	storageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.lastPath = r.URL.Path
		capture.lastMethod = r.Method
		capture.lastTeam = r.Header.Get(internalauth.TeamIDHeader)
		capture.callCount++

		token := r.Header.Get(internalauth.DefaultTokenHeader)
		validator := newValidator(t, "storage-proxy", keys.publicKey, []string{"internal-gateway"})
		if _, err := validator.Validate(token); err != nil {
			t.Fatalf("validate storage-proxy token: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	t.Cleanup(storageServer.Close)

	env := newGatewayTestEnv(t, managerServer.URL, storageServer.URL, nil, keys)
	return env, capture
}

func TestInternalGatewayIntegration_ProxyToStorageProxy(t *testing.T) {
	env, capture := newStorageProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxVolumeRead})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxvolumes", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/sandboxvolumes" {
		t.Fatalf("expected storage-proxy path /sandboxvolumes, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %s", capture.lastMethod)
	}
	if capture.lastTeam != "team-1" {
		t.Fatalf("expected team header team-1, got %s", capture.lastTeam)
	}
}

func TestInternalGatewayIntegration_SandboxVolumeCreate(t *testing.T) {
	env, capture := newStorageProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxVolumeCreate})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxvolumes", token, map[string]any{
		"name": "vol-1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/sandboxvolumes" {
		t.Fatalf("expected storage-proxy path /sandboxvolumes, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxVolumeGet(t *testing.T) {
	env, capture := newStorageProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxVolumeRead})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxvolumes/vol-1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/sandboxvolumes/vol-1" {
		t.Fatalf("expected storage-proxy path /sandboxvolumes/vol-1, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxVolumeDelete(t *testing.T) {
	env, capture := newStorageProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxVolumeDelete})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodDelete, env.server.URL+"/api/v1/sandboxvolumes/vol-1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/sandboxvolumes/vol-1" {
		t.Fatalf("expected storage-proxy path /sandboxvolumes/vol-1, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodDelete {
		t.Fatalf("expected method DELETE, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxVolumeSnapshotCreate(t *testing.T) {
	env, capture := newStorageProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxVolumeWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxvolumes/vol-1/snapshots", token, map[string]any{
		"name": "snap-1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/sandboxvolumes/vol-1/snapshots" {
		t.Fatalf("expected storage-proxy path /sandboxvolumes/vol-1/snapshots, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxVolumeSnapshotList(t *testing.T) {
	env, capture := newStorageProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxVolumeRead})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxvolumes/vol-1/snapshots", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/sandboxvolumes/vol-1/snapshots" {
		t.Fatalf("expected storage-proxy path /sandboxvolumes/vol-1/snapshots, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxVolumeSnapshotGet(t *testing.T) {
	env, capture := newStorageProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxVolumeRead})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxvolumes/vol-1/snapshots/snap-1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/sandboxvolumes/vol-1/snapshots/snap-1" {
		t.Fatalf("expected storage-proxy path /sandboxvolumes/vol-1/snapshots/snap-1, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxVolumeSnapshotRestore(t *testing.T) {
	env, capture := newStorageProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxVolumeWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxvolumes/vol-1/snapshots/snap-1/restore", token, map[string]any{
		"target": "vol-1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/sandboxvolumes/vol-1/snapshots/snap-1/restore" {
		t.Fatalf("expected storage-proxy path /sandboxvolumes/vol-1/snapshots/snap-1/restore, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxVolumeSnapshotDelete(t *testing.T) {
	env, capture := newStorageProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxVolumeDelete})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodDelete, env.server.URL+"/api/v1/sandboxvolumes/vol-1/snapshots/snap-1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/sandboxvolumes/vol-1/snapshots/snap-1" {
		t.Fatalf("expected storage-proxy path /sandboxvolumes/vol-1/snapshots/snap-1, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodDelete {
		t.Fatalf("expected method DELETE, got %s", capture.lastMethod)
	}
}
