package content

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// GetArticleType returns one nondeleted article type projection.
func (module *Module) GetArticleType(ctx context.Context, query GetArticleType) (ArticleTypeResult, error) {
	id, err := domain.NewArticleTypeID(query.ID)
	if err != nil {
		return ArticleTypeResult{}, validation(err)
	}
	view, err := module.taxonomies.FindArticleType(ctx, id)
	if err != nil {
		return ArticleTypeResult{}, stable(err)
	}
	return articleTypeViewResult(view), nil
}

// ListArticleTypes returns nondeleted article type projections.

func (module *Module) ListArticleTypes(ctx context.Context, query ListArticleTypes) (ArticleTypePage, error) {
	if err := validTaxonomyPage(query.Page, query.PageSize, query.Sort); err != nil {
		return ArticleTypePage{}, validation(err)
	}
	views, err := module.taxonomies.ListArticleTypes(ctx, query.Name)
	if err != nil {
		return ArticleTypePage{}, stable(err)
	}
	items := make([]ArticleTypeResult, len(views))
	for index, view := range views {
		items[index] = articleTypeViewResult(view)
	}
	sortArticleTypes(items, query.Sort)
	start, end := pageRange(len(items), query.Page, query.PageSize)
	return ArticleTypePage{Items: items[start:end], Number: query.Page, Size: query.PageSize, TotalItems: int64(len(items)), TotalPages: pages(len(items), query.PageSize)}, nil
}
