package rest

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
)

type loginTestUsers struct {
	account *user.User
	err     error
}

func (reader loginTestUsers) FindActiveByUsername(context.Context, string) (*user.User, error) {
	return reader.account, reader.err
}

type loginTestIssuer struct {
	token     string
	expiresAt time.Time
}

func (issuer loginTestIssuer) Issue(identityauth.Principal) (string, time.Time, error) {
	return issuer.token, issuer.expiresAt, nil
}

func TestLoginHandler_returnsGeneratedBearerResponse(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	id, err := user.ParseID("00000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	account, err := user.Reconstitute(id, "admin", string(hash), true, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	service, err := identityauth.NewLoginService(loginTestUsers{account: account}, loginTestIssuer{token: "signed-token", expiresAt: expiresAt})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewLoginHandler(service)
	if err != nil {
		t.Fatal(err)
	}

	response, err := handler.Login(context.Background(), generated.LoginRequestObject{Body: &generated.LoginJSONRequestBody{Username: " admin ", Password: "secret"}})
	if err != nil {
		t.Fatal(err)
	}
	loginResponse, ok := response.(generated.Login200JSONResponse)
	if !ok || loginResponse.AccessToken != "signed-token" || loginResponse.TokenType != generated.Bearer || !loginResponse.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("response=%#v", response)
	}
}

func TestLoginHandler_collapsesInvalidCredentialsToStableAuthenticationError(t *testing.T) {
	service, err := identityauth.NewLoginService(loginTestUsers{err: user.ErrNotFound}, loginTestIssuer{token: "unused", expiresAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewLoginHandler(service)
	if err != nil {
		t.Fatal(err)
	}

	_, err = handler.Login(context.Background(), generated.LoginRequestObject{Body: &generated.LoginJSONRequestBody{Username: "admin", Password: "wrong"}})
	if err == nil {
		t.Fatal("invalid credentials returned nil error")
	}
	var failure interface{ StableCode() string }
	if !errors.As(err, &failure) || failure.StableCode() != "unauthenticated" {
		t.Fatalf("error=%T %v", err, err)
	}
}

func TestLoginHandler_rejectsNilGeneratedBodyWithoutPanic(t *testing.T) {
	service, err := identityauth.NewLoginService(loginTestUsers{err: user.ErrNotFound}, loginTestIssuer{token: "unused", expiresAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewLoginHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	_, err = handler.Login(context.Background(), generated.LoginRequestObject{})
	var failure interface{ StableCode() string }
	if !errors.As(err, &failure) || failure.StableCode() != "unauthenticated" {
		t.Fatalf("error=%T %v", err, err)
	}
}
