package domain

import "time"

// ArticleImageState 是从可信持久化层重建图片所需的完整状态。
type ArticleImageState struct {
	Metadata    ArticleImageMetadata
	Status      ArticleImageStatus
	CreatedAt   time.Time
	CommittedAt time.Time
	OrphanedAt  time.Time
	ExpiresAt   time.Time
}

// ReconstituteArticleImage 校验数据库状态并重建图片聚合。
func ReconstituteArticleImage(state ArticleImageState) (*ArticleImage, error) {
	image, err := NewArticleImage(state.Metadata, state.CreatedAt.UTC(), state.ExpiresAt.UTC())
	if err != nil {
		return nil, err
	}
	switch state.Status {
	case ArticleImageStatusPending:
		if !state.CommittedAt.IsZero() || !state.OrphanedAt.IsZero() || state.ExpiresAt.Before(state.OrphanedAt) {
			return nil, invalid("image_timestamps")
		}
	case ArticleImageStatusCommitted:
		if state.CommittedAt.Before(state.CreatedAt) || state.CommittedAt.IsZero() || !state.OrphanedAt.IsZero() || state.ExpiresAt.Before(state.OrphanedAt) {
			return nil, invalid("image_timestamps")
		}
	case ArticleImageStatusOrphaned:
		if state.CommittedAt.Before(state.CreatedAt) || state.CommittedAt.IsZero() || state.OrphanedAt.Before(state.CommittedAt) || state.OrphanedAt.IsZero() || state.ExpiresAt.Before(state.OrphanedAt) {
			return nil, invalid("image_timestamps")
		}
	default:
		return nil, invalid("image_status")
	}
	image.status = state.Status
	image.committedAt = state.CommittedAt.UTC()
	image.orphanedAt = state.OrphanedAt.UTC()
	return image, nil
}

// Metadata 返回不可变图片元数据。
func (image *ArticleImage) Metadata() ArticleImageMetadata { return image.metadata }

// CreatedAt 返回创建时间。
func (image *ArticleImage) CreatedAt() time.Time { return image.createdAt }

// CommittedAt 返回首次确认时间。
func (image *ArticleImage) CommittedAt() time.Time { return image.committedAt }

// OrphanedAt 返回孤儿化时间。
func (image *ArticleImage) OrphanedAt() time.Time { return image.orphanedAt }

// ExpiresAt 返回当前过期时间。
func (image *ArticleImage) ExpiresAt() time.Time { return image.expiresAt }
