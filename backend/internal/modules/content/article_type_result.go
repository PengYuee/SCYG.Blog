package content

import "time"

// ArticleTypeResult 是协议无关的文章分类读取结果。
type ArticleTypeResult struct {
	// ID 是分类 ID。
	ID int64
	// Name 是分类名称。
	Name string
	// Image 是可选分类图片。
	Image *string
	// Meun 是稳定的菜单排序字段名。
	Meun int32
	// Version 是乐观锁版本。
	Version uint64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// ModifiedAt 是最后修改时间。
	ModifiedAt time.Time
}

// ArticleTypePage 是协议无关的文章分类分页结果。
type ArticleTypePage struct {
	// Items 是当前页分类。
	Items []ArticleTypeResult
	// Number 是当前页码。
	Number int
	// Size 是当前页大小。
	Size int
	// TotalItems 是符合条件的总条数。
	TotalItems int64
	// TotalPages 是符合条件的总页数。
	TotalPages int
}
