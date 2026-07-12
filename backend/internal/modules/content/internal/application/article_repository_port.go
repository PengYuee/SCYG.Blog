package application

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// ArticleRepository 以乐观锁版本持久化文章聚合。
type ArticleRepository interface {
	// NextID 在当前事务内预留标识符。
	NextID(context.Context) (domain.ArticleID, error)
	Find(context.Context, domain.ArticleID) (*domain.Article, error)
	Save(context.Context, *domain.Article) error
}
