package domain

import (
	"fmt"
	"time"
)

const defaultArticleImageOrphanGrace = 24 * time.Hour

// ArticleImageMetadata 是创建图片聚合所需的已解析元数据。
type ArticleImageMetadata struct {
	ID      ArticleImageID
	OwnerID ImageOwnerID
	// StorageKey 是最终存储键。
	StorageKey StorageKey
	MediaType  MediaType
	ByteSize   int64
	Width      int
	Height     int
	SHA256     string
}

// ArticleImage 维护图片确认与回收状态。
type ArticleImage struct {
	metadata    ArticleImageMetadata
	status      ArticleImageStatus
	createdAt   time.Time
	committedAt time.Time
	orphanedAt  time.Time
	expiresAt   time.Time
}

// NewArticleImage 创建待确认图片并校验跨字段不变量。
func NewArticleImage(metadata ArticleImageMetadata, createdAt, expiresAt time.Time) (*ArticleImage, error) {
	if metadata.ID.value == "" || metadata.OwnerID.value == "" || metadata.StorageKey.value == "" || !isLowerHex(metadata.SHA256, 64) {
		return nil, invalid("image_metadata")
	}
	if metadata.MediaType != MediaTypeJPEG && metadata.MediaType != MediaTypePNG {
		return nil, invalid("media_type")
	}
	if metadata.ByteSize < 1 || metadata.Width < 1 || metadata.Height < 1 {
		return nil, invalid("image_dimensions")
	}
	if createdAt.IsZero() || !expiresAt.After(createdAt) {
		return nil, invalid("image_timestamps")
	}
	return &ArticleImage{metadata: metadata, status: ArticleImageStatusPending, createdAt: createdAt, expiresAt: expiresAt}, nil
}

// Commit 将待确认图片或宽限期内孤儿图片确认为正式图片。
func (image *ArticleImage) Commit(at time.Time) error {
	if at.Before(image.createdAt) {
		return ErrTimeRegression
	}
	switch image.status {
	case ArticleImageStatusPending:
		if at.After(image.expiresAt) {
			return fmt.Errorf("待确认图片已过期：%w", ErrInvalidTransition)
		}
		image.status, image.committedAt = ArticleImageStatusCommitted, at
	case ArticleImageStatusOrphaned:
		if at.Before(image.orphanedAt) {
			return ErrTimeRegression
		}
		if at.After(image.expiresAt) {
			return fmt.Errorf("图片宽限期已过：%w", ErrInvalidTransition)
		}
		image.status, image.orphanedAt = ArticleImageStatusCommitted, time.Time{}
	case ArticleImageStatusCommitted:
		return fmt.Errorf("图片已经确认：%w", ErrInvalidTransition)
	}
	return nil
}

// Orphan 使用既有默认宽限期将正式图片置入待回收状态。
func (image *ArticleImage) Orphan(at time.Time) error {
	return image.OrphanWithGrace(at, defaultArticleImageOrphanGrace)
}

// OrphanWithGrace 使用注入宽限期将正式图片置入待回收状态。
func (image *ArticleImage) OrphanWithGrace(at time.Time, grace time.Duration) error {
	if image.status != ArticleImageStatusCommitted || at.Before(image.committedAt) {
		return fmt.Errorf("图片不能孤儿化：%w", ErrInvalidTransition)
	}
	image.status, image.orphanedAt, image.expiresAt = ArticleImageStatusOrphaned, at, at.Add(grace)
	return nil
}

// Cancel 使用既有默认宽限期取消待确认图片。
func (image *ArticleImage) Cancel(at time.Time) error {
	return image.CancelWithGrace(at, defaultArticleImageOrphanGrace)
}

// CancelWithGrace 使用注入宽限期将待确认图片置为可回收状态。
func (image *ArticleImage) CancelWithGrace(at time.Time, grace time.Duration) error {
	if image.status == ArticleImageStatusOrphaned {
		return nil
	}
	if image.status != ArticleImageStatusPending || at.Before(image.createdAt) {
		return fmt.Errorf("图片不能取消：%w", ErrInvalidTransition)
	}
	image.status, image.committedAt, image.orphanedAt, image.expiresAt = ArticleImageStatusOrphaned, at, at, at.Add(grace)
	return nil
}

// Status 返回当前图片状态。
func (image *ArticleImage) Status() ArticleImageStatus { return image.status }
