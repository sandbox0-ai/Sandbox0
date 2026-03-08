package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/sandbox0-ai/sandbox0/pkg/apispec"
)

const contentTypeBinary = "application/octet-stream"

func (s *Session) CreateDirectory(ctx context.Context, t ContractT, sandboxID, dirPath string, recursive bool) (int, error) {
	specPath := "/api/v1/sandboxes/{id}/files"
	query := url.Values{}
	query.Set("path", dirPath)
	query.Set("mkdir", "true")
	if recursive {
		query.Set("recursive", "true")
	}
	requestPath := "/api/v1/sandboxes/" + sandboxID + "/files?" + query.Encode()
	status, body, err := s.doRawSpecRequest(t, ctx, http.MethodPost, specPath, requestPath, nil, contentTypeBinary, defaultContentType, true)
	if err != nil {
		return status, err
	}
	if status != http.StatusCreated {
		return status, fmt.Errorf("create directory failed with status %d: %s", status, formatAPIError(body))
	}
	return status, nil
}

func (s *Session) WriteFile(ctx context.Context, t ContractT, sandboxID, filePath string, content []byte, contentType string) (int, error) {
	if strings.TrimSpace(contentType) == "" {
		contentType = contentTypeBinary
	}
	specPath := "/api/v1/sandboxes/{id}/files"
	requestPath := "/api/v1/sandboxes/" + sandboxID + "/files?path=" + url.QueryEscape(filePath)
	status, body, err := s.doRawSpecRequest(t, ctx, http.MethodPost, specPath, requestPath, content, contentType, defaultContentType, true)
	if err != nil {
		return status, err
	}
	if status != http.StatusOK {
		return status, fmt.Errorf("write file failed with status %d: %s", status, formatAPIError(body))
	}
	return status, nil
}

func (s *Session) ReadFile(ctx context.Context, t ContractT, sandboxID, filePath string) ([]byte, int, error) {
	specPath := "/api/v1/sandboxes/{id}/files"
	requestPath := "/api/v1/sandboxes/" + sandboxID + "/files?path=" + url.QueryEscape(filePath)
	status, body, err := s.doRawSpecRequest(t, ctx, http.MethodGet, specPath, requestPath, nil, "", contentTypeBinary, true)
	if err != nil {
		return nil, status, err
	}
	if status != http.StatusOK {
		return nil, status, fmt.Errorf("read file failed with status %d: %s", status, formatAPIError(body))
	}
	return body, status, nil
}

func (s *Session) ListFiles(ctx context.Context, t ContractT, sandboxID, dirPath string) (*apispec.SuccessFileListResponse, int, error) {
	specPath := "/api/v1/sandboxes/{id}/files/list"
	requestPath := "/api/v1/sandboxes/" + sandboxID + "/files/list?path=" + url.QueryEscape(dirPath)
	status, body, err := s.doRawSpecRequest(t, ctx, http.MethodGet, specPath, requestPath, nil, "", defaultContentType, true)
	if err != nil {
		return nil, status, err
	}
	if status != http.StatusOK {
		return nil, status, fmt.Errorf("list files failed with status %d: %s", status, formatAPIError(body))
	}
	var resp apispec.SuccessFileListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, status, fmt.Errorf("decode list files response: %w", err)
	}
	if !resp.Success {
		return nil, status, fmt.Errorf("list files response indicates failure")
	}
	return &resp, status, nil
}
