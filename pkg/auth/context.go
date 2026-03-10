// Package auth provides a compatibility shim around pkg/gateway/authn.
package auth

import (
	"context"

	"github.com/sandbox0-ai/sandbox0/pkg/gateway/authn"
)

type AuthContext = authn.AuthContext
type AuthMethod = authn.AuthMethod
type Principal = authn.Principal
type PrincipalKind = authn.PrincipalKind

const (
	AuthMethodAPIKey   = authn.AuthMethodAPIKey
	AuthMethodJWT      = authn.AuthMethodJWT
	AuthMethodInternal = authn.AuthMethodInternal

	PrincipalKindHuman    = authn.PrincipalKindHuman
	PrincipalKindAPIKey   = authn.PrincipalKindAPIKey
	PrincipalKindInternal = authn.PrincipalKindInternal

	PermSandboxCreate = authn.PermSandboxCreate
	PermSandboxRead   = authn.PermSandboxRead
	PermSandboxWrite  = authn.PermSandboxWrite
	PermSandboxDelete = authn.PermSandboxDelete

	PermTemplateCreate = authn.PermTemplateCreate
	PermTemplateRead   = authn.PermTemplateRead
	PermTemplateWrite  = authn.PermTemplateWrite
	PermTemplateDelete = authn.PermTemplateDelete

	PermSandboxVolumeCreate = authn.PermSandboxVolumeCreate
	PermSandboxVolumeRead   = authn.PermSandboxVolumeRead
	PermSandboxVolumeWrite  = authn.PermSandboxVolumeWrite
	PermSandboxVolumeDelete = authn.PermSandboxVolumeDelete
)

var RolePermissions = authn.RolePermissions

func WithAuthContext(ctx context.Context, authCtx *AuthContext) context.Context {
	return authn.WithAuthContext(ctx, authCtx)
}

func FromContext(ctx context.Context) *AuthContext {
	return authn.FromContext(ctx)
}

func ExpandRolePermissions(role string) []string {
	return authn.ExpandRolePermissions(role)
}

func ExpandRolesPermissions(roles []string) []string {
	return authn.ExpandRolesPermissions(roles)
}
