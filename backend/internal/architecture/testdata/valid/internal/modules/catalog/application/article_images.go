package application

import (
	catalog "example.com/architecture-valid/internal/modules/catalog"
	"example.com/architecture-valid/internal/modules/catalog/article"
	"example.com/architecture-valid/internal/modules/catalog/taxonomy"
)

type ArticleImages struct {
	Articles article.Service
	Taxonomy taxonomy.Service
}

func (ArticleImages) Create(catalog.FindArticle) (catalog.ArticleResult, error) {
	return catalog.ArticleResult{}, nil
}
