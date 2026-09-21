// Package user owns persisted user identities and password verification.
package user

import "errors"

// ErrNotFound indicates that no active user matched the query.
var ErrNotFound = errors.New("user not found")
