package user

import (
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrInvalidUser indicates that a user value violates its invariants.
	ErrInvalidUser = errors.New("invalid user")
	// ErrInvalidCredentials indicates that a password does not match.
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// ID is the stable 32-character lowercase hexadecimal user identifier.
type ID string

// ParseID validates a persisted user identifier.
func ParseID(raw string) (ID, error) {
	if len(raw) != 32 {
		return "", ErrInvalidUser
	}
	for _, value := range raw {
		if (value < '0' || value > '9') && (value < 'a' || value > 'f') {
			return "", ErrInvalidUser
		}
	}
	return ID(raw), nil
}

// String returns the persisted identifier.
func (id ID) String() string { return string(id) }

// User is an authenticated account.
type User struct {
	ID           ID
	Username     string
	passwordHash string
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    *time.Time
}

// Reconstitute rebuilds a user from its persisted credential state.
func Reconstitute(id ID, username, passwordHash string, active bool, createdAt time.Time, updatedAt *time.Time) (*User, error) {
	value := &User{ID: id, Username: username, passwordHash: passwordHash, Active: active, CreatedAt: createdAt, UpdatedAt: updatedAt}
	if err := validateUser(*value); err != nil {
		return nil, err
	}
	return value, nil
}

// VerifyPassword checks a plaintext password against the stored bcrypt hash.
func (value *User) VerifyPassword(password string) error {
	if value == nil || !value.Active || bcrypt.CompareHashAndPassword([]byte(value.passwordHash), []byte(password)) != nil {
		return ErrInvalidCredentials
	}
	return nil
}

func validateUser(value User) error {
	if _, err := ParseID(value.ID.String()); err != nil || strings.TrimSpace(value.Username) == "" || len(value.Username) > 64 || value.passwordHash == "" || value.CreatedAt.IsZero() {
		return ErrInvalidUser
	}
	return nil
}
