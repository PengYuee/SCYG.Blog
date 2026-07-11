// Package catalog owns REST-specific consumer interfaces.
package catalog

import (
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/catalog"
)

// ArticleFinder is the narrow interface consumed by this REST adapter.
type ArticleFinder interface {
	Find(catalog.FindArticle) (catalog.ArticleResult, error)
}
