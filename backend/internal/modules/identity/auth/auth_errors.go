// Package auth provides credential login, principals, and bearer token services.
package auth

import "errors"

var (
	// ErrInvalidCredentials is returned for every failed username/password login.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidToken indicates that a token cannot be trusted.
	ErrInvalidToken = errors.New("invalid token")
)
