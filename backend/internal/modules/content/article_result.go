package content

import "time"

// ArticleResult 是协议无关的文章读取结果，所有文本已通过领域规则校验。
type ArticleResult struct {
	// ID 是文章 ID。
	ID int64
	// ArticleTypeID 是所属分类 ID。
	ArticleTypeID int64
	// Title 是文章标题。
	Title string
	// Slug 是文章 URL 标识。
	Slug string
	// Digest 是文章摘要。
	Digest string
	// Content 是文章正文。
	Content string
	// Status 是稳定生命周期状态值。
	Status string
	// TagIDs 是关联标签 ID。
	TagIDs []int64
	// Support 是点赞计数。
	Support int64
	// Comment 是评论计数。
	Comment int64
	// Visited 是访问计数。
	Visited int64
	// Version 是乐观锁版本。
	Version uint64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// ModifiedAt 是最后修改时间。
	ModifiedAt time.Time
}

// ArticlePage 是协议无关的文章分页结果。
type ArticlePage struct {
	// Items 是当前页文章。
	Items []ArticleResult
	// Number 是当前页码。
	Number int
	// Size 是当前页大小。
	Size int
	// TotalItems 是符合条件的总条数。
	TotalItems int64
	// TotalPages 是符合条件的总页数。
	TotalPages int
}
