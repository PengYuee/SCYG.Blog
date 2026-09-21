package taxonomy

import (
	"context"
)

// GetArticleType reads one non-deleted article type.
func (service *Service) GetArticleType(ctx context.Context, query GetArticleType) (ArticleTypeResult, error) {
	if err := parseID(query.ID, "article_type_id"); err != nil {
		return ArticleTypeResult{}, err
	}
	entity, err := service.repo.internal().findArticleType(ctx, query.ID, false)
	if err != nil {
		return ArticleTypeResult{}, stable(err)
	}
	return articleTypeResult(entity), nil
}

// ListArticleTypes reads filtered and paginated article types.
func (service *Service) ListArticleTypes(ctx context.Context, query ListArticleTypes) (ArticleTypePage, error) {
	if err := validPage(query.Page, query.PageSize, query.Sort); err != nil {
		return ArticleTypePage{}, err
	}
	items, total, err := service.repo.internal().listArticleTypes(ctx, query.Name, query.Page, query.PageSize, query.Sort)
	if err != nil {
		return ArticleTypePage{}, stable(err)
	}
	result := make([]ArticleTypeResult, len(items))
	for index, item := range items {
		result[index] = articleTypeResult(item)
	}
	return ArticleTypePage{Items: result, Number: query.Page, Size: query.PageSize, TotalItems: total, TotalPages: pageCount(total, query.PageSize)}, nil
}
