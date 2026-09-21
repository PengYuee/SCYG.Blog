package user

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestParseID_acceptsLowerHexOnly(t *testing.T) {
	valid, err := ParseID("00000000000000000000000000000001")
	if err != nil || valid.String() != "00000000000000000000000000000001" {
		t.Fatalf("valid id = %q, err = %v", valid, err)
	}
	for _, value := range []string{"", "1", "0000000000000000000000000000000G", "00000000000000000000000000000001 "} {
		if _, err := ParseID(value); !errors.Is(err, ErrInvalidUser) {
			t.Fatalf("ParseID(%q) error = %v", value, err)
		}
	}
}

func TestUserVerifyPassword_requiresActiveMatchingBcryptHash(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	value := &User{ID: ID("00000000000000000000000000000001"), Username: "admin", passwordHash: string(hash), Active: true, CreatedAt: time.Now()}
	if err := value.VerifyPassword("secret"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(value.VerifyPassword("wrong"), ErrInvalidCredentials) {
		t.Fatal("wrong password was accepted")
	}
	value.Active = false
	if !errors.Is(value.VerifyPassword("secret"), ErrInvalidCredentials) {
		t.Fatal("inactive user was accepted")
	}
}

func TestNewRepository_rejectsNilDatabase(t *testing.T) {
	if _, err := NewRepository(nil); err == nil {
		t.Fatal("nil database was accepted")
	}
}
