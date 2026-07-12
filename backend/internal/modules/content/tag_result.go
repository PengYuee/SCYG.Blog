package content

import "time"

// TagResult 是协议无关的标签读取结果。
type TagResult struct {
	// ID 是标签 ID。
	ID int64
	// Name 是标签名称。
	Name string
	// Version 是乐观锁版本。
	Version uint64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// ModifiedAt 是最后修改时间。
	ModifiedAt time.Time
}

// TagPage 是协议无关的标签分页结果。
type TagPage struct {
	// Items 是当前页标签。
	Items []TagResult
	// Number 是当前页码。
	Number int
	// Size 是当前页大小。
	Size int
	// TotalItems 是符合条件的总条数。
	TotalItems int64
	// TotalPages 是符合条件的总页数。
	TotalPages int
}
