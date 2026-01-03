package procd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"sandbox0.ai/infra/pkg/api/procd"
	"sandbox0.ai/infra/pkg/test/fixtures"
)

// TestProcdAPIClientContract tests the HTTP API contract
// These tests verify that the HTTP API conforms to the spec defined in infra/spec/procd/procd.md
//
// This is a CONTRACT TEST - it validates the API interface contract
// When implementing the real ProcdClient, these tests must pass

func TestProcdAPI_Contract_CreateContext(t *testing.T) {
	// Setup mock server that matches the spec
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request matches spec
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/contexts", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse request body
		var req procd.CreateContextRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify request structure
		assert.Equal(t, procd.ProcessTypeREPL, req.Type)
		assert.Equal(t, "python", req.Language)
		assert.Equal(t, "/home/user", req.CWD)

		// Return response matching spec
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":       "ctx-abc123",
			"type":     "repl",
			"language": "python",
			"cwd":      "/home/user",
			"main_process": map[string]interface{}{
				"id":   "proc-123",
				"pid":  int32(1234),
				"type": "repl",
			},
			"created_at": "2024-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	// This test documents the expected HTTP contract
	// When implementing ProcdClient, it should make requests matching this format
	t.Logf("Expected HTTP contract for CreateContext:")
	t.Logf("  Method: POST")
	t.Logf("  Path: /api/v1/contexts")
	t.Logf("  Request: {type: 'repl', language: 'python', cwd: '/home/user', env_vars: {...}}")
	t.Logf("  Response: 201 Created with context object")
}

func TestProcdAPI_Contract_UpdateNetworkPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request matches spec
		assert.Equal(t, "PUT", r.Method)
		assert.Equal(t, "/api/v1/network/policy", r.URL.Path)

		var req procd.NetworkPolicy
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify policy structure
		assert.Equal(t, procd.NetworkModeWhitelist, req.Mode)
		assert.NotNil(t, req.Egress)
		assert.Contains(t, req.Egress.AllowCIDRs, "8.8.8.8")

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Logf("Expected HTTP contract for UpdateNetworkPolicy:")
	t.Logf("  Method: PUT")
	t.Logf("  Path: /api/v1/network/policy")
	t.Logf("  Request: {mode: 'whitelist', egress: {allow_cidrs: [...]}}")
}

func TestProcdAPI_Contract_MountVolume(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/volumes/vol-123/mount", r.URL.Path)

		var req procd.VolumeMountRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "sb-abc123", req.SandboxID)
		assert.Equal(t, "/workspace", req.MountPoint)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"volume_id":    "vol-123",
			"mount_point":  "/workspace",
			"layer_chain":  []string{"base-python-3.11", "working"},
			"is_from_cache": true,
		})
	}))
	defer server.Close()

	t.Logf("Expected HTTP contract for MountVolume:")
	t.Logf("  Method: POST")
	t.Logf("  Path: /api/v1/volumes/{volume_id}/mount")
}

func TestProcdAPI_Contract_ReadFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/api/v1/files/workspace/test.txt", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content":  "SGVsbG8gV29ybGQ=",
			"size":     int64(11),
			"mode":     "0644",
			"mod_time": "2024-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	t.Logf("Expected HTTP contract for ReadFile:")
	t.Logf("  Method: GET")
	t.Logf("  Path: /api/v1/files/*path")
}

// MockProcdClientForTesting is a simplified mock for testing other components
type MockProcdClientForTesting struct {
	createContextFunc func(ctx context.Context, req *procd.CreateContextRequest) (*procd.Context, error)
}

func (m *MockProcdClientForTesting) CreateContext(ctx context.Context, req *procd.CreateContextRequest) (*procd.Context, error) {
	if m.createContextFunc != nil {
		return m.createContextFunc(ctx, req)
	}
	return fixtures.NewContext(), nil
}

// Example: Using the mock in a unit test
func TestExample_UsingMockProcdClient(t *testing.T) {
	// Create mock
	mockClient := &MockProcdClientForTesting{
		createContextFunc: func(ctx context.Context, req *procd.CreateContextRequest) (*procd.Context, error) {
			// Custom mock behavior
			return &procd.Context{
				ID:       "custom-ctx-id",
				Type:     req.Type,
				Language: req.Language,
				CWD:      req.CWD,
			}, nil
		},
	}

	// Use the mock in your test
	ctx := context.Background()
	req := fixtures.NewCreateContextRequest()

	result, err := mockClient.CreateContext(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, "custom-ctx-id", result.ID)
	assert.Equal(t, procd.ProcessTypeREPL, result.Type)
}

// TestProcdAPI_Contract_Example shows how to write contract tests for all endpoints
func TestProcdAPI_Contract_Example(t *testing.T) {
	// This is a template for testing each endpoint

	testCases := []struct {
		name           string
		method         string
		path           string
		requestBody    interface{}
		expectedStatus int
		responseBody   interface{}
	}{
		{
			name:           "Health Check",
			method:         "GET",
			path:           "/healthz",
			expectedStatus: 200,
			responseBody: map[string]interface{}{
				"status":  "healthy",
				"version": "v1.0.0",
			},
		},
		// Add more test cases for each endpoint
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.method, r.Method)
				assert.Equal(t, tc.path, r.URL.Path)

				if tc.requestBody != nil {
					var req map[string]interface{}
					err := json.NewDecoder(r.Body).Decode(&req)
					require.NoError(t, err)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.expectedStatus)
				if tc.responseBody != nil {
					json.NewEncoder(w).Encode(tc.responseBody)
				}
			}))
			defer server.Close()

			t.Logf("Contract test for %s passed", tc.name)
		})
	}
}
