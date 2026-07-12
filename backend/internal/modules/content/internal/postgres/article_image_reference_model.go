package postgres

import "time"

// articleImageReferenceModel 是文章图片引用数据库模型。
type articleImageReferenceModel struct {
	ArticleID int64     `gorm:"column:article_id;primaryKey"`
	ImageID   string    `gorm:"column:image_id;primaryKey"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (articleImageReferenceModel) TableName() string { return "article_image_references" }
