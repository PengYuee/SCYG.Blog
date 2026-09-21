package config

import "time"

const developmentJWTSecret = "development-only-jwt-secret-change-me-32-bytes"

// Auth is immutable authentication configuration.
type Auth struct {
	jwtSecret      string
	issuer         string
	accessTokenTTL time.Duration
}

// JWTSecret returns the configured signing key for the authentication adapter.
func (auth Auth) JWTSecret() string { return auth.jwtSecret }

// Issuer returns the expected JWT issuer.
func (auth Auth) Issuer() string { return auth.issuer }

// AccessTokenTTL returns the access-token lifetime.
func (auth Auth) AccessTokenTTL() time.Duration { return auth.accessTokenTTL }
