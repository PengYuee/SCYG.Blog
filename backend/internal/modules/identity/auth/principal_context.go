package auth

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
)

// Principal is the authenticated identity carried by a request context.
type Principal struct {
	UserID   user.ID
	Username string
}

type principalContextKey struct{}

// WithPrincipal returns a context carrying an authenticated principal.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext returns the authenticated principal, if present.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	if ctx == nil {
		return Principal{}, false
	}
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}
