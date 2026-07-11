// Package application owns content use-case ports and transaction boundaries.
package application

import (
	"context"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// Clock supplies deterministic application time.
type Clock interface{ Now() time.Time }

// ArticleRepository persists article aggregates with optimistic versions.
type ArticleRepository interface {
	Find(context.Context, domain.ArticleID) (*domain.Article, error)
	Save(context.Context, *domain.Article) error
}

// ArticleTypeRepository persists article-type entities.
type ArticleTypeRepository interface {
	Find(context.Context, domain.ArticleTypeID) (*domain.ArticleType, error)
	Save(context.Context, *domain.ArticleType) error
}

// TagRepository persists tag entities.
type TagRepository interface {
	Find(context.Context, domain.TagID) (*domain.Tag, error)
	Save(context.Context, *domain.Tag) error
}

// ArticleReadModel serves public article projections without exposing persistence records.
type ArticleReadModel interface {
	FindPublished(context.Context, domain.ArticleID) (ArticleView, error)
	ListPublished(context.Context, ArticleFilter) (ArticlePage, error)
}

// TaxonomyReadModel serves article-type and tag projections.
type TaxonomyReadModel interface {
	ListArticleTypes(context.Context, string) ([]ArticleTypeView, error)
	ListTags(context.Context, string) ([]TagView, error)
}

// Transaction exposes transaction-scoped content ports only.
type Transaction interface {
	Articles() ArticleRepository
	ArticleTypes() ArticleTypeRepository
	Tags() TagRepository
}

// UnitOfWork runs a callback atomically without leaking transaction framework types.
type UnitOfWork interface {
	Within(context.Context, func(context.Context, Transaction) error) error
}

// ArticleFilter is a parsed query filter consumed by the read model.
type ArticleFilter struct {
	Page          int
	PageSize      int
	ArticleTypeID domain.ArticleTypeID
	TagID         domain.TagID
	Query         string
	Sort          string
}

// ArticleView is a protocol-neutral public projection.
type ArticleView struct {
	ID            domain.ArticleID
	ArticleTypeID domain.ArticleTypeID
	Title         domain.Title
	Slug          domain.Slug
	Digest        domain.Digest
	Status        domain.Status
	TagIDs        []domain.TagID
	Version       domain.Version
	CreatedAt     time.Time
	ModifiedAt    time.Time
}

// ArticlePage is a protocol-neutral paged projection.
type ArticlePage struct {
	Items      []ArticleView
	Number     int
	Size       int
	TotalItems int64
	TotalPages int
}

// ArticleTypeView is a protocol-neutral taxonomy projection.
type ArticleTypeView struct {
	ID      domain.ArticleTypeID
	Name    domain.Name
	Version domain.Version
}

// TagView is a protocol-neutral taxonomy projection.
type TagView struct {
	ID      domain.TagID
	Name    domain.Name
	Version domain.Version
}
