package internalgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mgrservice "github.com/sandbox0-ai/infra/manager/pkg/service"
	"github.com/sandbox0-ai/infra/pkg/auth"
	"github.com/sandbox0-ai/infra/pkg/internalauth"
)

type procdProxyCapture struct {
	lastPath   string
	lastQuery  string
	lastMethod string
	callCount  int
}

func newProcdProxyTestEnv(t *testing.T) (*gatewayTestEnv, *procdProxyCapture) {
	t.Helper()

	keys := gatewayKeyPair{}
	keys.privateKey, keys.publicKey = writeInternalGatewayKeys(t)

	capture := &procdProxyCapture{}
	procdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.lastPath = r.URL.Path
		capture.lastQuery = r.URL.RawQuery
		capture.lastMethod = r.Method
		capture.callCount++

		token := r.Header.Get(internalauth.DefaultTokenHeader)
		validator := newValidator(t, "procd", keys.publicKey, []string{"internal-gateway"})
		if _, err := validator.Validate(token); err != nil {
			t.Fatalf("validate procd token: %v", err)
		}

		procdToken := r.Header.Get(internalauth.TokenForProcdHeader)
		procdValidator := newValidator(t, "storage-proxy", keys.publicKey, []string{"procd"})
		if _, err := procdValidator.Validate(procdToken); err != nil {
			t.Fatalf("validate procd storage token: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	t.Cleanup(procdServer.Close)

	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/sandboxes/") {
			t.Fatalf("unexpected manager path: %s", r.URL.Path)
		}

		token := r.Header.Get(internalauth.DefaultTokenHeader)
		validator := newValidator(t, "manager", keys.publicKey, []string{"internal-gateway"})
		if _, err := validator.Validate(token); err != nil {
			t.Fatalf("validate manager token: %v", err)
		}

		sandbox := mgrservice.Sandbox{
			ID:           strings.TrimPrefix(r.URL.Path, "/api/v1/sandboxes/"),
			TeamID:       "team-1",
			UserID:       "user-1",
			ProcdAddress: procdServer.URL,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&sandbox)
	}))
	t.Cleanup(managerServer.Close)

	storageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected storage-proxy request: %s", r.URL.Path)
	}))
	t.Cleanup(storageServer.Close)

	env := newGatewayTestEnv(t, managerServer.URL, storageServer.URL, nil, keys)
	return env, capture
}

func TestInternalGatewayIntegration_ProxyToProcdExec(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/exec", token, map[string]any{
		"cmd": "echo ok",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/exec" {
		t.Fatalf("expected procd path /api/v1/exec, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_ProxyToProcdExecStream(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/exec/stream", token, map[string]any{
		"cmd": "echo ok",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/exec/stream" {
		t.Fatalf("expected procd path /api/v1/exec/stream, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_ContextCreate(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/contexts", token, map[string]any{
		"type": "bash",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/contexts" {
		t.Fatalf("expected procd path /api/v1/contexts, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_ContextList(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxes/sbx-1/contexts", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/contexts" {
		t.Fatalf("expected procd path /api/v1/contexts, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_ContextGet(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxes/sbx-1/contexts/ctx-1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/contexts/ctx-1" {
		t.Fatalf("expected procd path /api/v1/contexts/ctx-1, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_ContextDelete(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodDelete, env.server.URL+"/api/v1/sandboxes/sbx-1/contexts/ctx-1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/contexts/ctx-1" {
		t.Fatalf("expected procd path /api/v1/contexts/ctx-1, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodDelete {
		t.Fatalf("expected method DELETE, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_ContextRestart(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/contexts/ctx-1/restart", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/contexts/ctx-1/restart" {
		t.Fatalf("expected procd path /api/v1/contexts/ctx-1/restart, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_ContextExecute(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/contexts/ctx-1/execute", token, map[string]any{
		"cmd": "echo ok",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/contexts/ctx-1/execute" {
		t.Fatalf("expected procd path /api/v1/contexts/ctx-1/execute, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_ContextWebSocket(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, _ := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxes/sbx-1/contexts/ctx-1/ws", token, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected bad request for missing websocket upgrade, got %d", resp.StatusCode)
	}
	if capture.callCount != 0 {
		t.Fatalf("expected no procd calls for non-upgrade request")
	}
}

func TestInternalGatewayIntegration_FileStat(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxes/sbx-1/files/path/to/file?stat=true", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/files/path/to/file" {
		t.Fatalf("expected procd path /api/v1/files/path/to/file, got %s", capture.lastPath)
	}
	if capture.lastQuery != "stat=true" {
		t.Fatalf("expected query stat=true, got %s", capture.lastQuery)
	}
}

func TestInternalGatewayIntegration_FileList(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxes/sbx-1/files/path/to/dir?list=true", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/files/path/to/dir" {
		t.Fatalf("expected procd path /api/v1/files/path/to/dir, got %s", capture.lastPath)
	}
	if capture.lastQuery != "list=true" {
		t.Fatalf("expected query list=true, got %s", capture.lastQuery)
	}
}

func TestInternalGatewayIntegration_FileRead(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxes/sbx-1/files/path/to/file", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/files/path/to/file" {
		t.Fatalf("expected procd path /api/v1/files/path/to/file, got %s", capture.lastPath)
	}
}

func TestInternalGatewayIntegration_FileWrite(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/files/path/to/file", token, map[string]any{
		"content": "hello",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/files/path/to/file" {
		t.Fatalf("expected procd path /api/v1/files/path/to/file, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_FileMkdir(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/files/path/to/dir?mkdir=true", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/files/path/to/dir" {
		t.Fatalf("expected procd path /api/v1/files/path/to/dir, got %s", capture.lastPath)
	}
	if capture.lastQuery != "mkdir=true" {
		t.Fatalf("expected query mkdir=true, got %s", capture.lastQuery)
	}
}

func TestInternalGatewayIntegration_FileMove(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/files/move", token, map[string]any{
		"from": "/a",
		"to":   "/b",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/files/move" {
		t.Fatalf("expected procd path /api/v1/files/move, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_FileDelete(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodDelete, env.server.URL+"/api/v1/sandboxes/sbx-1/files/path/to/file", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/files/path/to/file" {
		t.Fatalf("expected procd path /api/v1/files/path/to/file, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodDelete {
		t.Fatalf("expected method DELETE, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxVolumeMount(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/sandboxvolumes/mount", token, map[string]any{
		"volume_id": "vol-1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxvolumes/mount" {
		t.Fatalf("expected procd path /api/v1/sandboxvolumes/mount, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxVolumeUnmount(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxWrite})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodPost, env.server.URL+"/api/v1/sandboxes/sbx-1/sandboxvolumes/unmount", token, map[string]any{
		"volume_id": "vol-1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxvolumes/unmount" {
		t.Fatalf("expected procd path /api/v1/sandboxvolumes/unmount, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", capture.lastMethod)
	}
}

func TestInternalGatewayIntegration_SandboxVolumeStatus(t *testing.T) {
	env, capture := newProcdProxyTestEnv(t)
	token := newInternalToken(t, env.edgeGen, []string{auth.PermSandboxRead})

	resp, body := doGatewayRequest(t, env.server.Client(), http.MethodGet, env.server.URL+"/api/v1/sandboxes/sbx-1/sandboxvolumes", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", resp.StatusCode, string(body))
	}
	if capture.lastPath != "/api/v1/sandboxvolumes/status" {
		t.Fatalf("expected procd path /api/v1/sandboxvolumes/status, got %s", capture.lastPath)
	}
	if capture.lastMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %s", capture.lastMethod)
	}
}
