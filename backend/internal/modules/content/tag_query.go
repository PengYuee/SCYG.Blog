package content

// ListTags 描述标签的协议无关筛选和分页条件。
type ListTags struct {
	// Page 是从 1 开始的页码。
	Page int
	// PageSize 是每页条数。
	PageSize int
	// Name 是可选名称筛选。
	Name string
	// Sort 是稳定排序标识。
	Sort string
}

// GetTag 描述查询一个未删除标签的输入。
type GetTag struct {
	// ID 是标签 ID。
	ID int64
}
