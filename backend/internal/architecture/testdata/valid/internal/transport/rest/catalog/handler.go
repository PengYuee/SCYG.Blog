// Package catalog owns REST-specific consumer interfaces.
package catalog

import (
	"example.com/architecture-valid/internal/modules/catalog"
)

// ArticleFinder is the narrow interface consumed by this REST adapter.
type ArticleFinder interface {
	Find(catalog.FindArticle) (catalog.ArticleResult, error)
}
