package http

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sandbox0-ai/infra/manager/pkg/apis/sandbox0/v1alpha1"
	"github.com/sandbox0-ai/infra/pkg/gateway/spec"
	"github.com/sandbox0-ai/infra/pkg/internalauth"
	"go.uber.org/zap"
)

// listTemplates lists available templates
func (s *Server) listTemplates(c *gin.Context) {
	// Get team ID from claims (optional, maybe filter by visibility later)
	claims := internalauth.ClaimsFromContext(c.Request.Context())
	if claims == nil {
		spec.JSONError(c, http.StatusUnauthorized, spec.CodeUnauthorized, "missing authentication")
		return
	}

	// Assuming we list all templates for now, or maybe filter by namespace if we had that concept exposed
	// For now, list all templates in the configured namespace (managed by service)
	// The service.ListTemplates(ctx, namespace) takes a namespace.
	// We might want to use the namespace from config, but Service usually deals with k8s namespaces.
	// Let's assume empty namespace lists all (if using lister) or we need to know the namespace.
	// Since manager manages a specific namespace usually (env.Namespace), we should probably use that.
	// But `TemplateService`'s `ListTemplates` with empty namespace lists all if using lister.

	templates, err := s.templateService.ListTemplates(c.Request.Context(), "")
	if err != nil {
		s.logger.Error("Failed to list templates", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to list templates: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusOK, gin.H{
		"templates": templates,
		"count":     len(templates),
	})
}

// getTemplate gets a template by ID
func (s *Server) getTemplate(c *gin.Context) {
	templateID := c.Param("id")
	if templateID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "template_id is required")
		return
	}

	// For now, assuming default namespace or we search.
	// Lister.Get requires namespace.
	// We'll iterate over all templates to find the one with the name if we don't know the namespace,
	// or assume a default namespace.
	// However, `TemplateService.GetTemplate` takes (namespace, id).
	// Let's try to find it in any namespace or use a configured one.
	// Ideally, the API should probably support namespacing, or the manager runs in a single namespace context.
	// Given `infra/manager/cmd/manager/main.go` uses `cfg.Namespace`, we should probably use that.
	// But `Server` doesn't have `cfg`.
	// For now, let's list all and find matching name, or better, inject default namespace into Server/Service.
	// Simplest: `ListTemplates` returns all, we filter by name.

	// Optimization: If we can't easily get the namespace, we can list all and find.
	// But `Get` is better.
	// Let's assume for now that templates are in the cluster-wide scope or we just list all and filter.
	// Actually `SandboxTemplate` is namespaced.
	// Let's try to fetch from all namespaces via Lister if possible, or just list and filter.

	templates, err := s.templateService.ListTemplates(c.Request.Context(), "")
	if err != nil {
		s.logger.Error("Failed to get template", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to get template: %v", err))
		return
	}

	var found *v1alpha1.SandboxTemplate
	for _, t := range templates {
		if t.Name == templateID {
			found = t
			break
		}
	}

	if found == nil {
		spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, "template not found")
		return
	}

	spec.JSONSuccess(c, http.StatusOK, found)
}

// createTemplate creates a new template
func (s *Server) createTemplate(c *gin.Context) {
	var template v1alpha1.SandboxTemplate
	if err := c.ShouldBindJSON(&template); err != nil {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	created, err := s.templateService.CreateTemplate(c.Request.Context(), &template)
	if err != nil {
		s.logger.Error("Failed to create template", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to create template: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusCreated, created)
}

// updateTemplate updates an existing template
func (s *Server) updateTemplate(c *gin.Context) {
	templateID := c.Param("id")
	if templateID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "template_id is required")
		return
	}

	var template v1alpha1.SandboxTemplate
	if err := c.ShouldBindJSON(&template); err != nil {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Ensure ID matches
	if template.Name != "" && template.Name != templateID {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "template_id in path does not match body")
		return
	}
	template.Name = templateID

	// Find existing to get namespace
	existingTemplates, err := s.templateService.ListTemplates(c.Request.Context(), "")
	if err != nil {
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, "failed to find existing template")
		return
	}

	var existing *v1alpha1.SandboxTemplate
	for _, t := range existingTemplates {
		if t.Name == templateID {
			existing = t
			break
		}
	}

	if existing == nil {
		spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, "template not found")
		return
	}

	template.Namespace = existing.Namespace

	updated, err := s.templateService.UpdateTemplate(c.Request.Context(), &template)
	if err != nil {
		s.logger.Error("Failed to update template", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to update template: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusOK, updated)
}

// deleteTemplate deletes a template
func (s *Server) deleteTemplate(c *gin.Context) {
	templateID := c.Param("id")
	if templateID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "template_id is required")
		return
	}

	// Find existing to get namespace
	existingTemplates, err := s.templateService.ListTemplates(c.Request.Context(), "")
	if err != nil {
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, "failed to find existing template")
		return
	}

	var namespace string
	found := false
	for _, t := range existingTemplates {
		if t.Name == templateID {
			namespace = t.Namespace
			found = true
			break
		}
	}

	if !found {
		// Already gone or not found
		spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, "template not found")
		return
	}

	err = s.templateService.DeleteTemplate(c.Request.Context(), namespace, templateID)
	if err != nil {
		s.logger.Error("Failed to delete template", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to delete template: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusOK, gin.H{"message": "template deleted"})
}

// WarmPoolRequest represents the request body for warming the pool
type WarmPoolRequest struct {
	Count int32 `json:"count"`
}

// warmPool warms the pool for a template
func (s *Server) warmPool(c *gin.Context) {
	templateID := c.Param("id")
	if templateID == "" {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, "template_id is required")
		return
	}

	var req WarmPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		spec.JSONError(c, http.StatusBadRequest, spec.CodeBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Find existing to get namespace
	existingTemplates, err := s.templateService.ListTemplates(c.Request.Context(), "")
	if err != nil {
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, "failed to find existing template")
		return
	}

	var namespace string
	found := false
	for _, t := range existingTemplates {
		if t.Name == templateID {
			namespace = t.Namespace
			found = true
			break
		}
	}

	if !found {
		spec.JSONError(c, http.StatusNotFound, spec.CodeNotFound, "template not found")
		return
	}

	err = s.templateService.WarmPool(c.Request.Context(), namespace, templateID, req.Count)
	if err != nil {
		s.logger.Error("Failed to warm pool", zap.Error(err))
		spec.JSONError(c, http.StatusInternalServerError, spec.CodeInternal, fmt.Sprintf("failed to warm pool: %v", err))
		return
	}

	spec.JSONSuccess(c, http.StatusOK, gin.H{"message": "pool warming triggered"})
}
