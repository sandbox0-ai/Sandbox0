package jwt

import "github.com/sandbox0-ai/sandbox0/pkg/gateway/authn"

type Claims = authn.Claims
type Issuer = authn.Issuer
type TokenPair = authn.TokenPair

var (
	ErrInvalidToken         = authn.ErrInvalidToken
	ErrTokenExpired         = authn.ErrTokenExpired
	ErrInvalidSigningMethod = authn.ErrInvalidSigningMethod
	ErrJWTNotConfigured     = authn.ErrJWTNotConfigured

	NewIssuer                = authn.NewIssuer
	GenerateRefreshTokenHash = authn.GenerateRefreshTokenHash
	HashRefreshToken         = authn.HashRefreshToken
)
