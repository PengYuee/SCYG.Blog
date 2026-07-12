package postgres

import "time"

// taxonomyProjectionRow 是分类与标签共享的投影数据模型。
type taxonomyProjectionRow struct {
	ID                   int64      `gorm:"column:Id"`
	Name                 string     `gorm:"column:Name"`
	Image                *string    `gorm:"column:Image"`
	Meun                 int32      `gorm:"column:Meun"`
	Version              int64      `gorm:"column:Version"`
	CreationTime         time.Time  `gorm:"column:CreationTime"`
	LastModificationTime *time.Time `gorm:"column:LastModificationTime"`
}
