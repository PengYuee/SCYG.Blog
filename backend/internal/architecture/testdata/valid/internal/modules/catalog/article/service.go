package article

import catalog "example.com/architecture-valid/internal/modules/catalog"

type Service struct{}

func (Service) Find(query catalog.FindArticle) (catalog.ArticleResult, error) {
	return catalog.ArticleResult{}, nil
}
