package content

// GetArticle 描述查询一篇公开文章的输入。
type GetArticle struct {
	// ID 是文章 ID。
	ID int64
}

// ListArticles 描述公开文章的分页与筛选条件，调用方必须提供已受限的页码参数。
type ListArticles struct {
	// Page 是从 1 开始的页码。
	Page int
	// PageSize 是每页条数。
	PageSize int
	// ArticleTypeID 是可选分类筛选。
	ArticleTypeID int64
	// TagID 是可选标签筛选。
	TagID int64
	// Query 是可选检索词。
	Query string
	// Sort 是稳定排序标识。
	Sort string
}
