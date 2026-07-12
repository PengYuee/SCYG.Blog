package content

// CreateArticleType 描述创建文章分类的输入；Meun 必须为非负值。
type CreateArticleType struct {
	// Name 是分类名称。
	Name string
	// Image 是可选分类图片。
	Image *string
	// Meun 是稳定的菜单排序字段名。
	Meun int32
}

// OptionalImage 区分未提供图片、设置图片和显式清空图片。
type OptionalImage struct {
	// Provided 表示请求是否包含 image 字段。
	Provided bool
	// Value 是提供的图片值，nil 表示显式清空。
	Value *string
}

// PatchArticleType 描述基于 ETag 版本局部更新文章分类的命令。
type PatchArticleType struct {
	// ID 是待更新分类 ID。
	ID int64
	// Version 是 If-Match 解析出的乐观锁版本。
	Version uint64
	// Name 是可选分类名称。
	Name *string
	// Image 保留 image 的三态语义。
	Image OptionalImage
	// Meun 是可选菜单排序。
	Meun *int32
}

// RenameArticleType 描述基于版本重命名文章分类的命令。
type RenameArticleType struct {
	// ID 是待重命名分类 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
	// Name 是新的分类名称。
	Name string
}

// DeleteArticleType 描述基于版本软删除文章分类的命令。
type DeleteArticleType struct {
	// ID 是待删除分类 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
}
