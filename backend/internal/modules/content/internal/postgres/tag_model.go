package postgres

import "time"

// tagModel 是标签持久化数据模型。
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
