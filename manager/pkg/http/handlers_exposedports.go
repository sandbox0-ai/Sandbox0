package http

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sandbox0-ai/infra/manager/pkg/service"
	"github.com/sandbox0-ai/infra/pkg/gateway/spec"
	"github.com/sandbox0-ai/infra/pkg/internalauth"
	"go.uber.org/zap"
)

type updateExposedPortsRequest struct {
	Ports []service.ExposedPortConfig `json:"ports"`
}

// getExposedPorts gets the exposed ports for a sandbox
func (s *Server) getExposedPorts(c *gin.Context) {
	sandboxID := c.Param("id")
	if sandboxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id is required")
		return
	}

	claims := internalauth.ClaimsFromContext(c.Request.Context())
	if claims == nil {
		spec.JSONError(c, http.StatusUnauthorized, spec.CodeUnauthorized, "missing authentication")
		return
	}

	sandbox, err := s.sandboxService.GetSandbox(c.Request.Context(), sandboxID)
	if err != nil {
		s.logger.Error("Failed to get sandbox",
			zap.String("sandboxID", sandboxID),
			zap.Error(err),
		)
		spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, fmt.Sprintf("sandbox not found: %v", err))
		return
	}

	if sandbox.TeamID != claims.TeamID {
		spec.JSONError(c, http.StatusForbidden, spec.CodeForbidden, "sandbox belongs to a different team")
		return
	}

	spec.JSONSuccess(c, http.StatusOK, gin.H{
		"sandbox_id":     sandboxID,
		"exposed_ports":  sandbox.ExposedPorts,
	})
}

// updateExposedPorts updates the exposed ports for a sandbox
func (s *Server) updateExposedPorts(c *gin.Context) {
	sandboxID := c.Param("id")
	if sandboxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id is required")
		return
	}

	claims := internalauth.ClaimsFromContext(c.Request.Context())
	if claims == nil {
		spec.JSONError(c, http.StatusUnauthorized, spec.CodeUnauthorized, "missing authentication")
		return
	}

	var req updateExposedPortsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	sandbox, err := s.sandboxService.GetSandbox(c.Request.Context(), sandboxID)
	if err != nil {
		spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, fmt.Sprintf("sandbox not found: %v", err))
		return
	}
	if sandbox.TeamID != claims.TeamID {
		spec.JSONError(c, http.StatusForbidden, spec.CodeForbidden, "sandbox belongs to a different team")
		return
	}

	cfg := &service.SandboxConfig{
		ExposedPorts: req.Ports,
	}
	updated, err := s.sandboxService.UpdateSandbox(c.Request.Context(), sandboxID, cfg)
	if err != nil {
		s.logger.Error("Failed to update exposed ports",
			zap.String("sandboxID", sandboxID),
			zap.Error(err),
		)
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to update exposed ports: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusOK, gin.H{
		"sandbox_id":     sandboxID,
		"exposed_ports":  updated.ExposedPorts,
	})
}

// clearExposedPorts clears all exposed ports for a sandbox
func (s *Server) clearExposedPorts(c *gin.Context) {
	sandboxID := c.Param("id")
	if sandboxID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id is required")
		return
	}

	claims := internalauth.ClaimsFromContext(c.Request.Context())
	if claims == nil {
		spec.JSONError(c, http.StatusUnauthorized, spec.CodeUnauthorized, "missing authentication")
		return
	}

	sandbox, err := s.sandboxService.GetSandbox(c.Request.Context(), sandboxID)
	if err != nil {
		spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, fmt.Sprintf("sandbox not found: %v", err))
		return
	}
	if sandbox.TeamID != claims.TeamID {
		spec.JSONError(c, http.StatusForbidden, spec.CodeForbidden, "sandbox belongs to a different team")
		return
	}

	cfg := &service.SandboxConfig{
		ExposedPorts: []service.ExposedPortConfig{},
	}
	updated, err := s.sandboxService.UpdateSandbox(c.Request.Context(), sandboxID, cfg)
	if err != nil {
		s.logger.Error("Failed to clear exposed ports",
			zap.String("sandboxID", sandboxID),
			zap.Error(err),
		)
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to clear exposed ports: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusOK, gin.H{
		"sandbox_id":     sandboxID,
		"exposed_ports":  updated.ExposedPorts,
	})
}

// deleteExposedPort deletes a specific exposed port for a sandbox
func (s *Server) deleteExposedPort(c *gin.Context) {
	sandboxID := c.Param("id")
	portStr := c.Param("port")
	if sandboxID == "" || portStr == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "sandbox_id and port are required")
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "invalid port number")
		return
	}

	claims := internalauth.ClaimsFromContext(c.Request.Context())
	if claims == nil {
		spec.JSONError(c, http.StatusUnauthorized, spec.CodeUnauthorized, "missing authentication")
		return
	}

	sandbox, err := s.sandboxService.GetSandbox(c.Request.Context(), sandboxID)
	if err != nil {
		spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, fmt.Sprintf("sandbox not found: %v", err))
		return
	}
	if sandbox.TeamID != claims.TeamID {
		spec.JSONError(c, http.StatusForbidden, spec.CodeForbidden, "sandbox belongs to a different team")
		return
	}

	// Remove the specific port
	newPorts := make([]service.ExposedPortConfig, 0, len(sandbox.ExposedPorts))
	found := false
	for _, p := range sandbox.ExposedPorts {
		if p.Port == port {
			found = true
			continue
		}
		newPorts = append(newPorts, p)
	}
	if !found {
		spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, "port not found in exposed ports")
		return
	}

	cfg := &service.SandboxConfig{
		ExposedPorts: newPorts,
	}
	updated, err := s.sandboxService.UpdateSandbox(c.Request.Context(), sandboxID, cfg)
	if err != nil {
		s.logger.Error("Failed to delete exposed port",
			zap.String("sandboxID", sandboxID),
			zap.Int("port", port),
			zap.Error(err),
		)
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to delete exposed port: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusOK, gin.H{
		"sandbox_id":     sandboxID,
		"exposed_ports":  updated.ExposedPorts,
	})
}
