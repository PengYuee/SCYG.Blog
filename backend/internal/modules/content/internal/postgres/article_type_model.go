package postgres

import "time"

// articleTypeModel 是文章分类持久化数据模型。
type articleTypeModel struct {
	ID                   int64      `gorm:"column:Id;primaryKey"`
	Name                 string     `gorm:"column:Name"`
	Image                *string    `gorm:"column:Image"`
	Meun                 int16      `gorm:"column:Meun"`
	Version              int64      `gorm:"column:Version"`
	CreationTime         time.Time  `gorm:"column:CreationTime"`
	LastModificationTime *time.Time `gorm:"column:LastModificationTime"`
	DeletionTime         *time.Time `gorm:"column:DeletionTime"`
	IsDeleted            bool       `gorm:"column:IsDeleted"`
}

func (articleTypeModel) TableName() string { return "ArticleType" }
