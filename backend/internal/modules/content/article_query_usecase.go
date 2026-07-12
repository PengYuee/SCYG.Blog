package content

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// GetArticle returns one published article projection.
func (module *Module) GetArticle(ctx context.Context, query GetArticle) (ArticleResult, error) {
	id, err := domain.NewArticleID(query.ID)
	if err != nil {
		return ArticleResult{}, validation(err)
	}
	view, err := module.articles.FindPublished(ctx, id)
	if err != nil {
		return ArticleResult{}, stable(err)
	}
	return articleViewResult(view), nil
}

// ListArticles returns a bounded page of published article projections.
func (module *Module) ListArticles(ctx context.Context, query ListArticles) (ArticlePage, error) {
	filter, err := parseFilter(query)
	if err != nil {
		return ArticlePage{}, validation(err)
	}
	page, err := module.articles.ListPublished(ctx, filter)
	if err != nil {
		return ArticlePage{}, stable(err)
	}
	items := make([]ArticleResult, len(page.Items))
	for index, item := range page.Items {
		items[index] = articleViewResult(item)
	}
	return ArticlePage{Items: items, Number: page.Number, Size: page.Size, TotalItems: page.TotalItems, TotalPages: page.TotalPages}, nil
}
