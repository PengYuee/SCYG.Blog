// Package authentication adapts authenticated principals to content policies.
package authentication

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
)

// ContextCurrentAuthor derives the content author from an authenticated request.
type ContextCurrentAuthor struct{}

// CurrentAuthor returns the authenticated subject as a content author ID.
func (ContextCurrentAuthor) CurrentAuthor(ctx context.Context) (content.AuthorID, error) {
	principal, ok := identityauth.PrincipalFromContext(ctx)
	if !ok {
		return content.AuthorID{}, content.CurrentAuthorUnavailableError{}
	}
	return content.NewAuthorID(principal.UserID.String())
}

// ContextAuthorizer permits protected content actions only for authenticated requests.
type ContextAuthorizer struct{}

// Authorize rejects requests without an authenticated principal.
func (ContextAuthorizer) Authorize(ctx context.Context, _ content.Action, _ content.Resource) error {
	if _, ok := identityauth.PrincipalFromContext(ctx); !ok {
		return content.ErrPermissionDenied
	}
	return nil
}
