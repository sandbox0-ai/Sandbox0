# Sandbox0 Infra - TDD Framework

This directory contains the test-driven development framework for sandbox0 infrastructure.

## Directory Structure

```
infra/
├── pkg/
│   ├── api/                      # Shared API interfaces (contract definitions)
│   │   ├── procd/
│   │   │   ├── types.go          # Data types for Procd API
│   │   │   ├── client.go         # ProcdClient interface
│   │   │   ├── client_mock.go    # Mock implementation
│   │   │   └── client_contract_test.go  # Contract tests
│   │   └── manager/
│   │       ├── types.go          # Data types for Manager API
│   │       ├── client.go         # ManagerClient interface
│   │       ├── client_mock.go    # Mock implementation
│   │       └── client_contract_test.go  # Contract tests
│   └── test/                     # Shared test utilities
│       ├── httputil/             # HTTP testing helpers
│       └── fixtures/             # Test data fixtures
├── procd/                        # Procd service implementation
├── manager/                      # Manager service implementation
└── internal-gateway/             # Internal Gateway implementation
```

## Development Workflow

### 1. Define Interface (Already Done)

The interfaces `ProcdClient` and `ManagerClient` are already defined based on the spec files.

### 2. Use Mocks for Parallel Development

When implementing a service that depends on another (e.g., internal-gateway depends on procd/manager), use the provided mocks:

```go
import (
    "sandbox0.ai/infra/pkg/api/procd"
    "sandbox0.ai/infra/pkg/api/manager"
    "sandbox0.ai/infra/pkg/test/fixtures"
)

func TestMyService(t *testing.T) {
    // Create mocks
    mockProcd := procd.NewMockProcdClient()
    mockManager := manager.NewMockManagerClient()

    // Setup expectations
    mockProcd.On("CreateContext", mock.Anything, mock.AnythingOfType("*procd.CreateContextRequest")).
        Return(fixtures.NewContext(), nil)

    // Use in your service
    service := NewMyService(mockProcd, mockManager)

    // Test...
    mockProcd.AssertExpectations(t)
}
```

### 3. Write Contract Tests

Each HTTP API endpoint should have a corresponding contract test that verifies:

1. Request method and path match the spec
2. Request body structure is correct
3. Response status code is correct
4. Response body structure matches the spec

See `pkg/api/procd/client_contract_test.go` for examples.

### 4. Implement Real Client

After writing tests, implement the real HTTP client:

```go
type HTTPProcdClient struct {
    client *http.Client
    baseURL string
}

func (c *HTTPProcdClient) CreateContext(ctx context.Context, req *CreateContextRequest) (*Context, error) {
    // Implementation here
}
```

### 5. Run Contract Tests Against Real Implementation

```bash
# Start the real procd service
make run-procd

# Run contract tests
go test ./pkg/api/procd -v -run=Contract
```

## Test Categories

### Unit Tests

Test individual functions/components using mocks:

```go
func TestVolumeManager_Create(t *testing.T) {
    mockProcd := NewMockProcdClient()
    vm := NewVolumeManager(mockProcd)

    err := vm.Create(ctx, "vol-123")
    assert.NoError(t, err)
}
```

### Integration Tests

Test multiple components together (no mocks):

```go
func TestProcdService_Integration(t *testing.T) {
    // Start real procd server
    server := NewTestProcdServer()
    defer server.Close()

    // Create real client
    client := NewHTTPProcdClient(server.URL)

    // Test real interactions
    ctx, req := fixtures.NewCreateContextRequest()
    result, err := client.CreateContext(ctx, req)
    assert.NoError(t, err)
}
```

### Contract Tests

Verify API matches the spec:

```go
func TestProcdAPI_Contract_CreateContext(t *testing.T) {
    // Verify HTTP contract matches spec
}
```

## Test Fixtures

Use pre-defined fixtures to avoid repetitive test setup:

```go
import "sandbox0.ai/infra/pkg/test/fixtures"

// Use fixtures in tests
req := fixtures.NewCreateContextRequest()
expected := fixtures.NewContext()
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test ./... -cover

# Run with race detection
go test ./... -race

# Run specific test
go test ./pkg/api/procd -v -run=TestCreateContext
```

## Next Steps

1. **Phase 1**: Implement procd or manager (can be done in parallel using mocks)
2. **Phase 2**: Implement internal-gateway using the mocks
3. **Phase 3**: Replace mocks with real implementations and run E2E tests
