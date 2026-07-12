package application

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// ArticleTypeRepository 持久化文章分类实体。
type ArticleTypeRepository interface {
	// NextID 在当前事务内预留标识符。
	NextID(context.Context) (domain.ArticleTypeID, error)
	Find(context.Context, domain.ArticleTypeID) (*domain.ArticleType, error)
	Save(context.Context, *domain.ArticleType) error
}

// TagRepository 持久化标签实体。
type TagRepository interface {
	// NextID 在当前事务内预留标识符。
	NextID(context.Context) (domain.TagID, error)
	Find(context.Context, domain.TagID) (*domain.Tag, error)
	Save(context.Context, *domain.Tag) error
}
