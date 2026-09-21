package authentication

import (
	"context"
	"testing"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
)

func TestContextCurrentAuthor_readsPrincipalSubject(t *testing.T) {
	principal := identityauth.Principal{}
	principal.UserID, _ = identityauthTestID()
	ctx := identityauth.WithPrincipal(context.Background(), principal)
	author, err := (ContextCurrentAuthor{}).CurrentAuthor(ctx)
	if err != nil || author.String() != principal.UserID.String() {
		t.Fatalf("author=%s err=%v", author.String(), err)
	}
}

func TestContextAuthorizer_requiresPrincipal(t *testing.T) {
	if err := (ContextAuthorizer{}).Authorize(context.Background(), "content.article.create", content.Resource{Kind: "article"}); err == nil {
		t.Fatal("unauthenticated request was authorized")
	}
}

func identityauthTestID() (user.ID, error) {
	return user.ParseID("00000000000000000000000000000001")
}
