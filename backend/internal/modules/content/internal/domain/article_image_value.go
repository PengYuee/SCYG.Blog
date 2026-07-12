package domain

import (
	"fmt"
	"strings"
)

// ArticleImageID 是严格的 32 位小写十六进制图片标识。
type ArticleImageID struct{ value string }

// ImageOwnerID 是严格的 32 位小写十六进制图片所有者标识。
type ImageOwnerID struct{ value string }

// StorageKey 是服务器生成且带安全图片扩展名的存储键。
type StorageKey struct{ value string }

// NewArticleImageID 解析图片标识。
func NewArticleImageID(raw string) (ArticleImageID, error) {
	if !isLowerHex(raw, 32) {
		return ArticleImageID{}, invalid("image_id")
	}
	return ArticleImageID{raw}, nil
}

// NewImageOwnerID 解析图片所有者标识，避免与身份任务的 AuthorID 冲突。
func NewImageOwnerID(raw string) (ImageOwnerID, error) {
	if !isLowerHex(raw, 32) {
		return ImageOwnerID{}, invalid("owner_id")
	}
	return ImageOwnerID{raw}, nil
}

// NewStorageKey 解析本站图片存储键。
func NewStorageKey(raw string) (StorageKey, error) {
	if len(raw) != 36 || !isLowerHex(raw[:32], 32) || raw[32] != '.' || (raw[33:] != "jpg" && raw[33:] != "png") {
		return StorageKey{}, invalid("storage_key")
	}
	return StorageKey{raw}, nil
}

// String 返回图片标识文本。
func (value ArticleImageID) String() string { return value.value }

// String 返回所有者标识文本。
func (value ImageOwnerID) String() string { return value.value }

// String 返回存储键文本。
func (value StorageKey) String() string { return value.value }

func isLowerHex(raw string, size int) bool {
	if len(raw) != size {
		return false
	}
	return strings.IndexFunc(raw, func(value rune) bool { return !(value >= '0' && value <= '9' || value >= 'a' && value <= 'f') }) == -1
}

// ArticleImageStatus 表示图片持久生命周期。
type ArticleImageStatus string

const (
	// ArticleImageStatusPending 表示尚未被文章确认的图片。
	ArticleImageStatusPending ArticleImageStatus = "pending"
	// ArticleImageStatusCommitted 表示至少曾被文章确认的图片。
	ArticleImageStatusCommitted ArticleImageStatus = "committed"
	// ArticleImageStatusOrphaned 表示已进入回收宽限期的图片。
	ArticleImageStatusOrphaned ArticleImageStatus = "orphaned"
)

// NewArticleImageStatus 解析闭合状态集合。
func NewArticleImageStatus(raw string) (ArticleImageStatus, error) {
	status := ArticleImageStatus(raw)
	switch status {
	case ArticleImageStatusPending, ArticleImageStatusCommitted, ArticleImageStatusOrphaned:
		return status, nil
	default:
		return "", fmt.Errorf("status %q: %w", raw, ErrInvalidValue)
	}
}

// MediaType 是服务端解码确认的图片媒体类型。
type MediaType string

const (
	// MediaTypeJPEG 表示 JPEG 图片。
	MediaTypeJPEG MediaType = "image/jpeg"
	// MediaTypePNG 表示 PNG 图片。
	MediaTypePNG MediaType = "image/png"
)
