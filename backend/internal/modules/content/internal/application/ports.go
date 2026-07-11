// Package application 定义内容用例端口和事务边界。
package application

import (
	"context"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// Clock 为应用层提供确定性时间。
type Clock interface{ Now() time.Time }

// ArticleRepository 以乐观锁版本持久化文章聚合。
type ArticleRepository interface {
	// NextID 在当前事务内预留标识符。
	NextID(context.Context) (domain.ArticleID, error)
	Find(context.Context, domain.ArticleID) (*domain.Article, error)
	Save(context.Context, *domain.Article) error
}

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

// ArticleReadModel 提供公开文章投影而不泄露持久化记录。
type ArticleReadModel interface {
	FindPublished(context.Context, domain.ArticleID) (ArticleView, error)
	ListPublished(context.Context, ArticleFilter) (ArticlePage, error)
}

// ArticleAdminReadModel 提供显式非公开文章投影。
type ArticleAdminReadModel interface {
	ListAll(context.Context, ArticleFilter) (ArticlePage, error)
}

// TaxonomyReadModel 提供文章分类和标签投影。
type TaxonomyReadModel interface {
	FindArticleType(context.Context, domain.ArticleTypeID) (ArticleTypeView, error)
	ListArticleTypes(context.Context, string) ([]ArticleTypeView, error)
	FindTag(context.Context, domain.TagID) (TagView, error)
	ListTags(context.Context, string) ([]TagView, error)
}

// Transaction 仅暴露事务范围内的内容端口，仓储不得跨该范围泄露。
type Transaction interface {
	Articles() ArticleRepository
	ArticleTypes() ArticleTypeRepository
	Tags() TagRepository
}

// UnitOfWork 在不泄露事务框架类型的前提下原子执行回调；成功才提交，错误必须回滚。
type UnitOfWork interface {
	Within(context.Context, func(context.Context, Transaction) error) error
}

// ArticleFilter 是读模型消费的已解析查询筛选条件。
type ArticleFilter struct {
	// Page 是从 1 开始的页码。
	Page int
	// PageSize 是每页条数。
	PageSize int
	// ArticleTypeID 是可选文章分类筛选。
	ArticleTypeID domain.ArticleTypeID
	// TagID 是可选标签筛选。
	TagID domain.TagID
	// Query 是可选检索词。
	Query string
	// Status 是可选生命周期状态筛选。
	Status domain.Status
	// Sort 是稳定排序标识。
	Sort string
}

// ArticleView 是协议无关的公开文章投影。
type ArticleView struct {
	// ID 是文章 ID。
	ID domain.ArticleID
	// ArticleTypeID 是所属分类 ID。
	ArticleTypeID domain.ArticleTypeID
	// Title 是文章标题。
	Title domain.Title
	// Slug 是文章 URL 标识。
	Slug domain.Slug
	// Digest 是文章摘要。
	Digest domain.Digest
	// Content 是文章正文。
	Content domain.Content
	// Status 是文章生命周期状态。
	Status domain.Status
	// TagIDs 是关联标签 ID。
	TagIDs []domain.TagID
	// Support 是点赞计数。
	Support int64
	// Comment 是评论计数。
	Comment int64
	// Visited 是访问计数。
	Visited int64
	// Version 是乐观锁版本。
	Version domain.Version
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// ModifiedAt 是最后修改时间。
	ModifiedAt time.Time
}

// ArticlePage 是协议无关的文章分页投影。
type ArticlePage struct {
	// Items 是当前页文章。
	Items []ArticleView
	// Number 是当前页码。
	Number int
	// Size 是当前页大小。
	Size int
	// TotalItems 是符合条件的总条数。
	TotalItems int64
	// TotalPages 是符合条件的总页数。
	TotalPages int
}

// ArticleTypeView 是协议无关的文章分类投影。
type ArticleTypeView struct {
	// ID 是分类 ID。
	ID domain.ArticleTypeID
	// Name 是分类名称。
	Name domain.Name
	// Image 是可选分类图片。
	Image *string
	// Meun 是稳定的菜单排序字段名。
	Meun int32
	// Version 是乐观锁版本。
	Version domain.Version
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// ModifiedAt 是最后修改时间。
	ModifiedAt time.Time
}

// TagView 是协议无关的标签投影。
type TagView struct {
	// ID 是标签 ID。
	ID domain.TagID
	// Name 是标签名称。
	Name domain.Name
	// Version 是乐观锁版本。
	Version domain.Version
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// ModifiedAt 是最后修改时间。
	ModifiedAt time.Time
}
