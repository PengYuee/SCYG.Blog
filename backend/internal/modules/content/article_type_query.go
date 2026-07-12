package content

// ListArticleTypes 描述文章分类的协议无关筛选和分页条件。
type ListArticleTypes struct {
	// Page 是从 1 开始的页码。
	Page int
	// PageSize 是每页条数。
	PageSize int
	// Name 是可选名称筛选。
	Name string
	// Sort 是稳定排序标识。
	Sort string
}

// GetArticleType 描述查询一个未删除文章分类的输入。
type GetArticleType struct {
	// ID 是文章分类 ID。
	ID int64
}
