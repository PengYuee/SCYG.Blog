package content_test

import (
	"context"
	"errors"
	"testing"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func Test_Authorizer_DenyAll_rejects_every_action(t *testing.T) {
	actions := []content.Action{content.ActionCreateArticle, content.ActionReviseArticle, content.ActionPublishArticle, content.ActionArchiveArticle, content.ActionDeleteArticle, content.ActionManageArticleType, content.ActionManageTag}
	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			err := (content.DenyAll{}).Authorize(context.Background(), action, content.Resource{Kind: "article", ID: 1})
			if !errors.Is(err, content.ErrPermissionDenied) {
				t.Fatalf("expected deny-all, got %v", err)
			}
			var failure *content.ApplicationError
			if !errors.As(err, &failure) || failure.Code != content.CodePermissionDenied {
				t.Fatalf("expected typed permission error, got %v", err)
			}
		})
	}
}
