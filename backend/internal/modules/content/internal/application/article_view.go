package application

import (
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

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
