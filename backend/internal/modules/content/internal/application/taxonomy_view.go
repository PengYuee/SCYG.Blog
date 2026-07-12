package application

import (
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

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
