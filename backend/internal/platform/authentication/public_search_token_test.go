package authentication

import (
	"context"
	"strings"
	"testing"
	"time"

	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
)

func TestPublicSearchCredentialsStayOutsideAuthorDomain(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	secret := []byte(strings.Repeat("s", 32))
	anonymous, err := NewPublicSearchTokens(secret, "blog", time.Hour, clock)
	if err != nil {
		t.Fatal(err)
	}
	author, err := identityauth.NewTokenService(secret, "blog", time.Hour, clock)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := anonymous.Issue()
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := anonymous.Issue()
	if err != nil {
		t.Fatal(err)
	}
	firstSubject, err := anonymous.Verify(first)
	if err != nil {
		t.Fatal(err)
	}
	secondSubject, err := anonymous.Verify(second)
	if err != nil || firstSubject == secondSubject {
		t.Fatal("issuance reused anonymous identity")
	}
	if _, err := author.Verify(first); err == nil {
		t.Fatal("anonymous credential authenticated an author")
	}
	authorToken, _, err := author.Issue(identityauth.Principal{UserID: user.ID(strings.Repeat("a", 32)), Username: "author"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := anonymous.Verify(authorToken); err == nil {
		t.Fatal("author credential entered public credential domain")
	}
	ctx := WithAnonymousSubject(context.Background(), firstSubject)
	if _, ok := identityauth.PrincipalFromContext(ctx); ok {
		t.Fatal("anonymous identity installed an author principal")
	}
	now = now.Add(time.Hour)
	if _, err := anonymous.Verify(first); err == nil {
		t.Fatal("expired anonymous identity remained valid")
	}
}
