package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sandbox0-ai/infra/pkg/apispec"
)

func (s *Session) ListTemplates(ctx context.Context, t ContractT) ([]apispec.SandboxTemplate, error) {
	status, body, err := s.doJSONSpecRequest(t, ctx, http.MethodGet, "/api/v1/templates", "/api/v1/templates", nil, true)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list templates failed with status %d: %s", status, formatAPIError(body))
	}
	var resp apispec.SuccessTemplateListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if !resp.Success || resp.Data == nil || resp.Data.Templates == nil {
		return nil, nil
	}
	return *resp.Data.Templates, nil
}

func (s *Session) CreateTemplate(ctx context.Context, t ContractT, template apispec.SandboxTemplate) (*apispec.SandboxTemplate, error) {
	status, body, err := s.doJSONSpecRequest(t, ctx, http.MethodPost, "/api/v1/templates", "/api/v1/templates", template, true)
	if err != nil {
		return nil, err
	}
	if status != http.StatusCreated {
		return nil, fmt.Errorf("create template failed with status %d: %s", status, formatAPIError(body))
	}
	var resp apispec.SuccessTemplateResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if !resp.Success || resp.Data == nil {
		return nil, fmt.Errorf("create template response missing data")
	}
	return resp.Data, nil
}

func (s *Session) UpdateTemplate(ctx context.Context, t ContractT, templateID string, template apispec.SandboxTemplate) (*apispec.SandboxTemplate, error) {
	if templateID == "" {
		return nil, fmt.Errorf("template id is required")
	}
	specPath := "/api/v1/templates/{id}"
	requestPath := "/api/v1/templates/" + templateID
	status, body, err := s.doJSONSpecRequest(t, ctx, http.MethodPut, specPath, requestPath, template, true)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("update template failed with status %d: %s", status, formatAPIError(body))
	}
	var resp apispec.SuccessTemplateResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if !resp.Success || resp.Data == nil {
		return nil, fmt.Errorf("update template response missing data")
	}
	return resp.Data, nil
}

func (s *Session) DeleteTemplate(ctx context.Context, t ContractT, templateID string) error {
	if templateID == "" {
		return fmt.Errorf("template id is required")
	}
	specPath := "/api/v1/templates/{id}"
	requestPath := "/api/v1/templates/" + templateID
	status, body, err := s.doJSONSpecRequest(t, ctx, http.MethodDelete, specPath, requestPath, nil, true)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNotFound {
		return fmt.Errorf("delete template failed with status %d: %s", status, formatAPIError(body))
	}
	return nil
}

func CloneTemplateForCreate(base apispec.SandboxTemplate, name string) apispec.SandboxTemplate {
	template := apispec.SandboxTemplate{
		ApiVersion: cloneStringPtr(base.ApiVersion),
		Kind:       cloneStringPtr(base.Kind),
		Metadata:   cloneObjectMeta(base.Metadata, name),
		Spec:       cloneSandboxTemplateSpec(base.Spec),
	}
	return template
}

func cloneObjectMeta(base *apispec.ObjectMeta, name string) *apispec.ObjectMeta {
	if base == nil && name == "" {
		return nil
	}
	meta := &apispec.ObjectMeta{
		Name:        cloneStringPtr(&name),
		Namespace:   cloneStringPtr(ptrValue(base, func(v *apispec.ObjectMeta) *string { return v.Namespace })),
		Labels:      cloneStringMapPtr(ptrValueMap(base, func(v *apispec.ObjectMeta) *map[string]string { return v.Labels })),
		Annotations: cloneStringMapPtr(ptrValueMap(base, func(v *apispec.ObjectMeta) *map[string]string { return v.Annotations })),
	}
	return meta
}

func cloneSandboxTemplateSpec(base *apispec.SandboxTemplateSpec) *apispec.SandboxTemplateSpec {
	if base == nil {
		return nil
	}
	spec := *base
	return &spec
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneStringMapPtr(value *map[string]string) *map[string]string {
	if value == nil {
		return nil
	}
	copied := make(map[string]string, len(*value))
	for k, v := range *value {
		copied[k] = v
	}
	return &copied
}

func ptrValue[T any](value *T, getter func(*T) *string) *string {
	if value == nil {
		return nil
	}
	return getter(value)
}

func ptrValueMap[T any](value *T, getter func(*T) *map[string]string) *map[string]string {
	if value == nil {
		return nil
	}
	return getter(value)
}
