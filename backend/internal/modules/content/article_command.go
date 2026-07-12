package content

// CreateArticle 描述创建文章草稿所需的已解析输入。
type CreateArticle struct {
	// ArticleTypeID 是所属文章分类 ID。
	ArticleTypeID int64
	// Title 是文章标题。
	Title string
	// Slug 是文章的 URL 标识。
	Slug string
	// Digest 是文章摘要。
	Digest string
	// Content 是文章正文。
	Content string
	// TagIDs 是文章关联的标签 ID。
	TagIDs []int64
}

// ReviseArticle 描述基于乐观锁版本修订文章的完整输入。
type ReviseArticle struct {
	// ID 是待修订文章 ID。
	ID int64
	// Version 是调用方持有的乐观锁版本。
	Version uint64
	// ArticleTypeID 是目标文章分类 ID。
	ArticleTypeID int64
	// Title 是目标文章标题。
	Title string
	// Slug 是目标 URL 标识。
	Slug string
	// Digest 是目标摘要。
	Digest string
	// Content 是目标正文。
	Content string
	// TagIDs 是目标标签 ID 集合。
	TagIDs []int64
}

// PatchArticle 描述文章局部更新；nil 字段表示调用方未提供，Version 必须来自强 ETag。
type PatchArticle struct {
	// ID 是待更新文章 ID。
	ID int64
	// Version 是 If-Match 解析出的乐观锁版本。
	Version uint64
	// ArticleTypeID 为可选目标分类 ID。
	ArticleTypeID *int64
	// Title 为可选标题。
	Title *string
	// Slug 为可选 URL 标识。
	Slug *string
	// Digest 为可选摘要。
	Digest *string
	// Content 为可选正文。
	Content *string
	// TagIDs 为可选标签集合，非 nil 的空切片表示清空。
	TagIDs *[]int64
	// Status 为可选生命周期状态。
	Status *string
}

// PublishArticle 描述基于版本发布草稿的命令。
type PublishArticle struct {
	// ID 是待发布文章 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
}

// ArchiveArticle 描述基于版本归档已发布文章的命令。
type ArchiveArticle struct {
	// ID 是待归档文章 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
}

// DeleteArticle 描述基于版本软删除文章的命令。
type DeleteArticle struct {
	// ID 是待删除文章 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
}
