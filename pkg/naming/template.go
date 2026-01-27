package naming

import (
	"fmt"
	"strings"
)

const (
	ScopePublic = "public"
	ScopeTeam   = "team"
)

// TenantKey returns a stable short key for a team ID.
// It is intentionally short to keep derived Kubernetes resource names within limits.
func TenantKey(teamID string) string {
	return shortHash(teamID)
}

// TeamKey is an alias for TenantKey to keep naming consistent.
func TeamKey(teamID string) string {
	return TenantKey(teamID)
}

// TemplateNameForCluster returns a Kubernetes-safe name for storing a template in a cluster.
//
// For public templates, the name is the templateID.
// For team-scoped templates, the name includes a stable team key to avoid cross-tenant collisions.
func TemplateNameForCluster(scope, teamID, templateID string) string {
	if scope != ScopeTeam {
		name, err := slugWithHash(templateID, dnsLabelMaxLen)
		if err != nil {
			return fmt.Sprintf("tpl-%s", shortHash(templateID))
		}
		return name
	}

	teamKey := TeamKey(teamID)
	prefix := fmt.Sprintf("t-%s-", teamKey)
	remaining := dnsLabelMaxLen - len(prefix)
	if remaining <= 0 {
		return fmt.Sprintf("t-%s-%s", teamKey, shortHash(templateID))
	}
	name, err := slugWithHash(templateID, remaining)
	if err != nil {
		return fmt.Sprintf("t-%s-%s", teamKey, shortHash(templateID))
	}
	return prefix + name
}

// ValidateTemplateName ensures template name is non-empty and safe for storage.
func ValidateTemplateName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("template_name is required")
	}
	if len(trimmed) > 255 {
		return fmt.Errorf("template_name is too long (%d > 255)", len(trimmed))
	}
	if strings.Contains(trimmed, "/") {
		return fmt.Errorf("template_name cannot contain '/'")
	}
	return nil
}

// TemplateIDFromName generates a stable template_id from template_name.
func TemplateIDFromName(name string) (string, error) {
	if err := ValidateTemplateName(name); err != nil {
		return "", err
	}
	return slugWithHash(name, dnsLabelMaxLen)
}

// TemplateNamespaceFromName generates a DNS-safe namespace for a template name.
func TemplateNamespaceFromName(name string) (string, error) {
	return TemplateIDFromName(name)
}
