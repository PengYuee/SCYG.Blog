package postgres

// tagArticleModel 是文章标签关联持久化数据模型。
type tagArticleModel struct {
	ArticleID int64 `gorm:"column:ArticleId;primaryKey"`
	TagID     int64 `gorm:"column:TagId;primaryKey"`
}

func (tagArticleModel) TableName() string { return "TagArticle" }

// projectionTagRow 是文章标签关联查询数据模型。
type projectionTagRow struct {
	ArticleID int64 `gorm:"column:ArticleId"`
	TagID     int64 `gorm:"column:TagId"`
}
