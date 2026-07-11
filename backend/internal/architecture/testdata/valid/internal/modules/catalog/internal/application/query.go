// Package application owns catalog use cases and narrow ports.
package application

import (
	"context"

	"example.com/architecture-valid/internal/modules/catalog/internal/domain"
)

// ArticleReader is a consumer-owned persistence port.
type ArticleReader interface {
	Find(context.Context, int64) (domain.Article, error)
}

// ArticleRepository is a concrete business port, not a generic CRUD abstraction.
type ArticleRepository interface {
	FindPublished(context.Context, int64) (domain.Article, error)
}
