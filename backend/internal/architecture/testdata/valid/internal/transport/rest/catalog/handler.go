package catalog

import (
	"example.com/architecture-valid/internal/modules/catalog/application"
	"example.com/architecture-valid/internal/modules/catalog/article"
)

type Handler struct {
	workflow application.ArticleImages
	queries  article.Service
}
