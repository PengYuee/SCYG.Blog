package image

import "time"

type imageModel struct {
	ID                    string     `gorm:"column:id;primaryKey"`
	OwnerID               string     `gorm:"column:owner_id"`
	StorageKey            string     `gorm:"column:storage_key"`
	MediaType             string     `gorm:"column:media_type"`
	ByteSize              int64      `gorm:"column:byte_size"`
	Width                 int        `gorm:"column:width"`
	Height                int        `gorm:"column:height"`
	SHA256                string     `gorm:"column:sha256"`
	Status                string     `gorm:"column:status"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
	CommittedAt           *time.Time `gorm:"column:committed_at"`
	OrphanedAt            *time.Time `gorm:"column:orphaned_at"`
	ExpiresAt             time.Time  `gorm:"column:expires_at"`
	CleanupClaimToken     *string    `gorm:"column:cleanup_claim_token"`
	CleanupClaimExpiresAt *time.Time `gorm:"column:cleanup_claim_expires_at"`
}

func (imageModel) TableName() string { return "article_images" }

type referenceModel struct {
	ArticleID int64     `gorm:"column:article_id;primaryKey"`
	ImageID   string    `gorm:"column:image_id;primaryKey"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (referenceModel) TableName() string { return "article_image_references" }
func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copied := value.UTC()
	return &copied
}

func timeValue(value *time.Time, fallback time.Time) time.Time {
	if value == nil {
		return fallback
	}
	return value.UTC()
}
