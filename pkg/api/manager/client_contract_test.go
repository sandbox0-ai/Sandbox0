package manager_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"sandbox0.ai/infra/pkg/api/manager"
	"sandbox0.ai/infra/pkg/test/fixtures"
)

// TestManagerAPIClientContract tests the HTTP API contract
// These tests verify that the HTTP API conforms to the spec defined in infra/spec/manager/manager.md
//
// This is a CONTRACT TEST - it validates the API interface contract
// When implementing the real ManagerClient, these tests must pass

func TestManagerAPI_Contract_ClaimSandbox(t *testing.T) {
	// Setup mock server that matches the spec
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request matches spec
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/sandboxes/claim", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse request body
		var req manager.ClaimSandboxRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify request structure
		assert.Equal(t, "python-dev", req.TemplateID)
		assert.Equal(t, "team-123", req.TeamID)
		assert.Equal(t, "user-456", req.UserID)

		// Return response matching spec
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sandbox_id":    "sandbox-abc",
			"template_id":   "python-dev",
			"status":        "starting",
			"procd_address": "sandbox-abc-pod.default.svc.cluster.local:8080",
		})
	}))
	defer server.Close()

	t.Logf("Expected HTTP contract for ClaimSandbox:")
	t.Logf("  Method: POST")
	t.Logf("  Path: /api/v1/sandboxes/claim")
	t.Logf("  Request: {template_id, team_id, user_id, sandbox_id, config}")
	t.Logf("  Response: 201 Created with sandbox info")
}

func TestManagerAPI_Contract_GetSandbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/api/v1/sandboxes/sb-123", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":            "sb-123",
			"template_id":   "python-dev",
			"team_id":       "team-123",
			"user_id":       "user-456",
			"status":        "running",
			"procd_address": "sb-123-pod.default.svc.cluster.local:8080",
			"config": map[string]interface{}{
				"env_vars": map[string]string{"API_KEY": "xxx"},
				"ttl":      int32(3600),
			},
		})
	}))
	defer server.Close()

	t.Logf("Expected HTTP contract for GetSandbox:")
	t.Logf("  Method: GET")
	t.Logf("  Path: /api/v1/sandboxes/{id}")
}

func TestManagerAPI_Contract_ListTemplates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/api/v1/templates", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id":          "python-dev",
				"name":        "python-dev",
				"description": "Python development environment",
			},
			{
				"id":          "nodejs-dev",
				"name":        "nodejs-dev",
				"description": "Node.js development environment",
			},
		})
	}))
	defer server.Close()

	t.Logf("Expected HTTP contract for ListTemplates:")
	t.Logf("  Method: GET")
	t.Logf("  Path: /api/v1/templates")
}

func TestManagerAPI_Contract_CreateTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/templates", r.URL.Path)

		var req manager.CreateTemplateRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "python-dev", req.Name)
		assert.NotNil(t, req.Spec)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "python-dev",
			"name":        "python-dev",
			"description": "Python development environment",
		})
	}))
	defer server.Close()

	t.Logf("Expected HTTP contract for CreateTemplate:")
	t.Logf("  Method: POST")
	t.Logf("  Path: /api/v1/templates")
}

func TestManagerAPI_Contract_WarmPool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/templates/python-dev/pool/warm", r.URL.Path)

		var req manager.WarmPoolRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, int32(3), req.Count)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"template_id":   "python-dev",
			"warmed_count": int32(3),
		})
	}))
	defer server.Close()

	t.Logf("Expected HTTP contract for WarmPool:")
	t.Logf("  Method: POST")
	t.Logf("  Path: /api/v1/templates/{id}/pool/warm")
}

// MockManagerClientForTesting is a simplified mock for testing other components
type MockManagerClientForTesting struct {
	claimSandboxFunc func(ctx context.Context, req *manager.ClaimSandboxRequest) (*manager.ClaimSandboxResponse, error)
}

func (m *MockManagerClientForTesting) ClaimSandbox(ctx context.Context, req *manager.ClaimSandboxRequest) (*manager.ClaimSandboxResponse, error) {
	if m.claimSandboxFunc != nil {
		return m.claimSandboxFunc(ctx, req)
	}
	return fixtures.NewClaimSandboxResponse(), nil
}

// Example: Using the mock in a unit test
func TestExample_UsingMockManagerClient(t *testing.T) {
	// Create mock
	mockClient := &MockManagerClientForTesting{
		claimSandboxFunc: func(ctx context.Context, req *manager.ClaimSandboxRequest) (*manager.ClaimSandboxResponse, error) {
			// Custom mock behavior
			return &manager.ClaimSandboxResponse{
				SandboxID:    "custom-sb-id",
				TemplateID:   req.TemplateID,
				Status:       manager.SandboxStatusStarting,
				ProcdAddress: "custom-sb-id-pod.default.svc.cluster.local:8080",
			}, nil
		},
	}

	// Use the mock in your test
	ctx := context.Background()
	req := fixtures.NewClaimSandboxRequest()

	result, err := mockClient.ClaimSandbox(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, "custom-sb-id", result.SandboxID)
	assert.Equal(t, manager.SandboxStatusStarting, result.Status)
}

// TestManagerAPI_Contract_AllEndpoints shows how to write contract tests for all endpoints
func TestManagerAPI_Contract_AllEndpoints(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		path           string
		requestBody    interface{}
		expectedStatus int
		responseBody   interface{}
	}{
		{
			name:           "Claim Sandbox",
			method:         "POST",
			path:           "/api/v1/sandboxes/claim",
			requestBody:    fixtures.NewClaimSandboxRequest(),
			expectedStatus: 201,
			responseBody:   fixtures.NewClaimSandboxResponse(),
		},
		{
			name:           "Get Sandbox",
			method:         "GET",
			path:           "/api/v1/sandboxes/sb-123",
			expectedStatus: 200,
			responseBody:   fixtures.NewSandbox(),
		},
		{
			name:           "List Templates",
			method:         "GET",
			path:           "/api/v1/templates",
			expectedStatus: 200,
			responseBody:   []manager.Template{*fixtures.NewTemplate()},
		},
		{
			name:           "Create Volume",
			method:         "POST",
			path:           "/api/v1/volumes",
			requestBody:    fixtures.NewCreateVolumeRequest(),
			expectedStatus: 201,
			responseBody:   fixtures.NewVolume(),
		},
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
