package domain

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func Test_ValidateArticleImageWithLimits_rejects_custom_smaller_limits(t *testing.T) {
	// Given
	source := encodedPNG(t, 2, 2)
	tests := []struct {
		name   string
		limits ArticleImageValidationLimits
		target error
	}{
		{"文件字节", NewArticleImageValidationLimits(int64(len(source)-1), 4, 2), ErrArticleImageTooLarge},
		{"像素总数", NewArticleImageValidationLimits(int64(len(source)), 3, 2), ErrArticleImageDimensions},
		{"单边尺寸", NewArticleImageValidationLimits(int64(len(source)), 4, 1), ErrArticleImageDimensions},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// When
			_, err := ValidateArticleImageWithLimits(bytes.NewReader(source), testCase.limits)

			// Then
			if !errors.Is(err, testCase.target) {
				t.Fatalf("期望 %v，实际 %v", testCase.target, err)
			}
		})
	}
}

func Test_ArticleImage_orphan_restore_uses_custom_grace(t *testing.T) {
	// Given
	image, created, _ := pendingImage(t)
	if err := image.Commit(created.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	orphanedAt := created.Add(2 * time.Hour)
	const grace = 30 * time.Minute
	if err := image.OrphanWithGrace(orphanedAt, grace); err != nil {
		t.Fatal(err)
	}

	// When
	err := image.Commit(orphanedAt.Add(grace).Add(time.Nanosecond))

	// Then
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("超过自定义宽限期应拒绝恢复：%v", err)
	}
}

func Test_NewArticleImage_accepts_metadata_already_validated_by_configured_limits(t *testing.T) {
	// Given
	id, _ := NewArticleImageID("abcdef0123456789abcdef0123456789")
	owner, _ := NewImageOwnerID("0123456789abcdef0123456789abcdef")
	key, _ := NewStorageKey("abcdef0123456789abcdef0123456789.jpg")
	createdAt := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	metadata := ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: MediaTypeJPEG, ByteSize: MaxArticleImageBytes + 1, Width: MaxArticleImageDimension + 1, Height: 1, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}

	// When
	_, err := NewArticleImage(metadata, createdAt, createdAt.Add(time.Hour))
	// Then
	if err != nil {
		t.Fatalf("已由注入配置验证的元数据不应被默认常量再次拒绝：%v", err)
	}
}
