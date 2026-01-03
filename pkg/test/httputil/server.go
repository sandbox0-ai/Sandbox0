package httputil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertJSONContentType asserts that the response has JSON content type
func AssertJSONContentType(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

// AssertStatusCode asserts the response status code
func AssertStatusCode(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	assert.Equal(t, expected, rec.Code, "status code mismatch")
}

// ParseJSONBody parses the response body as JSON
func ParseJSONBody(t *testing.T, rec *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	err := json.NewDecoder(rec.Body).Decode(v)
	require.NoError(t, err, "failed to parse JSON response")
}

// ReadBody reads the response body
func ReadBody(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err, "failed to read response body")
	return string(body)
}

// NewJSONRequest creates a new HTTP request with JSON body
func NewJSONRequest(t *testing.T, method, url string, body interface{}) *http.Request {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		require.NoError(t, err, "failed to marshal request body")
		bodyReader = strings.NewReader(string(jsonData))
	}
	req, err := http.NewRequest(method, url, bodyReader)
	require.NoError(t, err, "failed to create request")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// AssertHeader asserts a response header value
func AssertHeader(t *testing.T, rec *httptest.ResponseRecorder, key, expected string) {
	t.Helper()
	actual := rec.Header().Get(key)
	assert.Equal(t, expected, actual, "header %s mismatch", key)
}

// MatchJSON is a custom matcher for JSON (useful for testify/assert)
func MatchJSON(t *testing.T, expected, actual interface{}) bool {
	t.Helper()
	expectedBytes, err := json.Marshal(expected)
	require.NoError(t, err, "failed to marshal expected JSON")
	actualBytes, err := json.Marshal(actual)
	require.NoError(t, err, "failed to marshal actual JSON")
	return assert.JSONEq(t, string(expectedBytes), string(actualBytes))
}
