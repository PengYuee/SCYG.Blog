package postgres

import "time"

// articleImageModel 是图片元数据数据库模型。
type articleImageModel struct {
	ID          string     `gorm:"column:id;primaryKey"`
	OwnerID     string     `gorm:"column:owner_id"`
	StorageKey  string     `gorm:"column:storage_key"`
	MediaType   string     `gorm:"column:media_type"`
	ByteSize    int64      `gorm:"column:byte_size"`
	Width       int        `gorm:"column:width"`
	Height      int        `gorm:"column:height"`
	SHA256      string     `gorm:"column:sha256"`
	Status      string     `gorm:"column:status"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	CommittedAt *time.Time `gorm:"column:committed_at"`
	OrphanedAt  *time.Time `gorm:"column:orphaned_at"`
	ExpiresAt   time.Time  `gorm:"column:expires_at"`
}

func (articleImageModel) TableName() string { return "article_images" }
