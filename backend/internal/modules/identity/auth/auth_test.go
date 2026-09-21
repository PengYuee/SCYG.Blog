package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
)

type fakeUsers struct {
	account *user.User
	err     error
}

func (fake fakeUsers) FindActiveByUsername(context.Context, string) (*user.User, error) {
	return fake.account, fake.err
}

func testTokenService(t *testing.T) *TokenService {
	t.Helper()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	service, err := NewTokenService([]byte("01234567890123456789012345678901"), "scyg-api", time.Hour, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func testUser(t *testing.T) *user.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	id, err := user.ParseID("00000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	account, err := user.Reconstitute(id, "admin", string(hash), true, time.Now().UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func TestLogin_returnsBearerTokenAndPrincipal(t *testing.T) {
	tokens := testTokenService(t)
	service, err := NewLoginService(fakeUsers{account: testUser(t)}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Login(context.Background(), "admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if result.TokenType != "Bearer" || result.AccessToken == "" || result.Principal.Username != "admin" {
		t.Fatalf("unexpected login result: %+v", result)
	}
	principal, verifyErr := tokens.Verify(result.AccessToken)
	if verifyErr != nil || principal != result.Principal {
		t.Fatalf("verified principal=%+v err=%v, want %+v", principal, verifyErr, result.Principal)
	}
}

func TestLogin_collapsesMissingAndWrongCredentials(t *testing.T) {
	tokens := testTokenService(t)
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "missing", err: user.ErrNotFound},
		{name: "repository invalid", err: user.ErrInvalidCredentials},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewLoginService(fakeUsers{err: test.err}, tokens)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Login(context.Background(), "admin", "secret"); !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("error=%v, want invalid credentials", err)
			}
		})
	}
	service, err := NewLoginService(fakeUsers{account: testUser(t)}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(context.Background(), "admin", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error=%v", err)
	}
}

func TestTokenService_rejectsTamperingAndWrongIssuer(t *testing.T) {
	service := testTokenService(t)
	id, err := user.ParseID("00000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := service.Issue(Principal{UserID: id, Username: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Verify(token + "x"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("tampered token error=%v", err)
	}
	other, err := NewTokenService([]byte("01234567890123456789012345678901"), "other-api", time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Verify(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong issuer token error=%v", err)
	}
}

func TestPrincipalContext_roundTrips(t *testing.T) {
	id, err := user.ParseID("00000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	want := Principal{UserID: id, Username: "admin"}
	got, ok := PrincipalFromContext(WithPrincipal(context.Background(), want))
	if !ok || got != want {
		t.Fatalf("principal=%+v present=%v, want %+v", got, ok, want)
	}
}
