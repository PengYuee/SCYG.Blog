package taxonomy

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// ListManageTags authorizes management access before executing a paginated query.
func (service *Service) ListManageTags(ctx context.Context, input ListTags) (TagPage, error) {
	if err := service.authorizer.Authorize(ctx, ActionManageTag, content.Resource{Kind: "tag"}); err != nil {
		return TagPage{}, permission()
	}
	return service.ListTags(ctx, input)
}

// ListManageArticleTypes authorizes before querying article classifications.
func (service *Service) ListManageArticleTypes(ctx context.Context, input ListArticleTypes) (ArticleTypePage, error) {
	if err := service.authorizer.Authorize(ctx, ActionManageArticleType, content.Resource{Kind: "article_type"}); err != nil {
		return ArticleTypePage{}, permission()
	}
	return service.ListArticleTypes(ctx, input)
}

// TagsByIDs resolves only the tags linked by a bounded article projection.
// The caller must already have authorized access to that article projection.
func (service *Service) TagsByIDs(ctx context.Context, ids []int64) ([]TagResult, error) {
	return service.repo.tagsByIDs(ctx, ids)
}
