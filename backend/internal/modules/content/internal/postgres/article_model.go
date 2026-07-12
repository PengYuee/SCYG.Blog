package postgres

import "time"

// articleModel 是文章持久化数据模型。
type articleModel struct {
	ID                   int64      `gorm:"column:Id;primaryKey"`
	ArticleTypeID        int64      `gorm:"column:ArticleTypeId"`
	Title                string     `gorm:"column:Title"`
	Slug                 string     `gorm:"column:Slug"`
	Digest               string     `gorm:"column:Digest"`
	Content              string     `gorm:"column:Content"`
	Status               int16      `gorm:"column:Status"`
	Support              int64      `gorm:"column:Support"`
	Comment              int64      `gorm:"column:Comment"`
	Visited              int64      `gorm:"column:Visited"`
	Version              int64      `gorm:"column:Version"`
	CreationTime         time.Time  `gorm:"column:CreationTime"`
	LastModificationTime *time.Time `gorm:"column:LastModificationTime"`
	DeletionTime         *time.Time `gorm:"column:DeletionTime"`
	IsDeleted            bool       `gorm:"column:IsDeleted"`
}

func (articleModel) TableName() string { return "Article" }

// articleProjectionRow 是文章查询数据模型。
type articleProjectionRow struct {
	ID                   int64      `gorm:"column:Id"`
	ArticleTypeID        int64      `gorm:"column:ArticleTypeId"`
	Title                string     `gorm:"column:Title"`
	Slug                 string     `gorm:"column:Slug"`
	Digest               string     `gorm:"column:Digest"`
	Content              string     `gorm:"column:Content"`
	Support              int64      `gorm:"column:Support"`
	Comment              int64      `gorm:"column:Comment"`
	Visited              int64      `gorm:"column:Visited"`
	Status               int16      `gorm:"column:Status"`
	Version              int64      `gorm:"column:Version"`
	CreationTime         time.Time  `gorm:"column:CreationTime"`
	LastModificationTime *time.Time `gorm:"column:LastModificationTime"`
}
