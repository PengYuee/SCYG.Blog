// Package application owns catalog use cases and narrow ports.
package application

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/catalog/internal/domain"
)

// ArticleReader is a consumer-owned persistence port.
type ArticleReader interface {
	Find(context.Context, int64) (domain.Article, error)
}
