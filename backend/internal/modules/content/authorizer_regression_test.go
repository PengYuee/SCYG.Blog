package content_test

import (
	"context"
	"errors"
	"testing"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

type customAuthorizer struct{}

func (*customAuthorizer) Authorize(context.Context, content.Action, content.Resource) error {
	return nil
}

func Test_AuthorizerOrDeny_normalizes_nil_and_typed_nil_to_deny_all(t *testing.T) {
	var typedNil *customAuthorizer
	for _, candidate := range []content.Authorizer{nil, typedNil} {
		normalized := content.AuthorizerOrDeny(candidate)
		err := normalized.Authorize(context.Background(), content.ActionCreateArticle, content.Resource{})
		if !errors.Is(err, content.ErrPermissionDenied) {
			t.Fatalf("default authorizer allowed request: %v", err)
		}
	}
}

func Test_AuthorizerOrDeny_preserves_real_authorizer(t *testing.T) {
	custom := &customAuthorizer{}
	if content.AuthorizerOrDeny(custom) != custom {
		t.Fatal("real authorizer was replaced")
	}
}
