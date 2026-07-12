package application

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// ArticleReadModel 提供公开文章投影而不泄露持久化记录。
type ArticleReadModel interface {
	FindPublished(context.Context, domain.ArticleID) (ArticleView, error)
	ListPublished(context.Context, ArticleFilter) (ArticlePage, error)
}

// ArticleAdminReadModel 提供显式非公开文章投影。
type ArticleAdminReadModel interface {
	ListAll(context.Context, ArticleFilter) (ArticlePage, error)
}
