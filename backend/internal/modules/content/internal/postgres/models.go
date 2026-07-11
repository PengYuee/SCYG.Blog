package postgres

import "time"

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

type tagModel struct {
	ID                   int64      `gorm:"column:Id;primaryKey"`
	Name                 string     `gorm:"column:Name"`
	Version              int64      `gorm:"column:Version"`
	CreationTime         time.Time  `gorm:"column:CreationTime"`
	LastModificationTime *time.Time `gorm:"column:LastModificationTime"`
	DeletionTime         *time.Time `gorm:"column:DeletionTime"`
	IsDeleted            bool       `gorm:"column:IsDeleted"`
}

func (tagModel) TableName() string { return "Tag" }

type tagArticleModel struct {
	ArticleID int64 `gorm:"column:ArticleId;primaryKey"`
	TagID     int64 `gorm:"column:TagId;primaryKey"`
}

func (tagArticleModel) TableName() string { return "TagArticle" }
