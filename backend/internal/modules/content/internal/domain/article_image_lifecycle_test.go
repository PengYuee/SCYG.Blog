package domain

import (
	"errors"
	"testing"
	"time"
)

func Test_ArticleImageValues_reject_noncanonical_input(t *testing.T) {
	tests := []struct {
		name  string
		parse func() error
	}{
		{"uppercase image id", func() error { _, err := NewArticleImageID("ABCDEF0123456789abcdef0123456789"); return err }},
		{"short owner id", func() error { _, err := NewImageOwnerID("abcdef"); return err }},
		{"storage key without extension", func() error { _, err := NewStorageKey("abcdef0123456789abcdef0123456789"); return err }},
		{"unknown status", func() error { _, err := NewArticleImageStatus("deleted"); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.parse(); !errors.Is(err, ErrInvalidValue) {
				t.Fatalf("期望值错误，实际 %v", err)
			}
		})
	}
}

func Test_ArticleImage_transitions_follow_lifecycle(t *testing.T) {
	id, _ := NewArticleImageID("abcdef0123456789abcdef0123456789")
	owner, _ := NewImageOwnerID("0123456789abcdef0123456789abcdef")
	key, _ := NewStorageKey("11111111111111111111111111111111.jpg")
	created := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	image, err := NewArticleImage(ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: MediaTypeJPEG, ByteSize: 128, Width: 10, Height: 20, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, created, created.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("创建图片失败：%v", err)
	}
	if err = image.Commit(created.Add(time.Hour)); err != nil {
		t.Fatalf("确认失败：%v", err)
	}
	if err = image.Orphan(created.Add(2 * time.Hour)); err != nil {
		t.Fatalf("孤儿化失败：%v", err)
	}
	if err = image.Commit(created.Add(3 * time.Hour)); err != nil {
		t.Fatalf("宽限期恢复失败：%v", err)
	}
	if image.Status() != ArticleImageStatusCommitted {
		t.Fatalf("期望 committed，实际 %s", image.Status())
	}
}

func Test_ArticleImage_rejects_illegal_transition(t *testing.T) {
	id, _ := NewArticleImageID("abcdef0123456789abcdef0123456789")
	owner, _ := NewImageOwnerID("0123456789abcdef0123456789abcdef")
	key, _ := NewStorageKey("11111111111111111111111111111111.png")
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	image, _ := NewArticleImage(ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: MediaTypePNG, ByteSize: 1, Width: 1, Height: 1, SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, now, now.Add(time.Hour))
	if err := image.Orphan(now.Add(time.Minute)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("期望非法迁移，实际 %v", err)
	}
}

func Test_ArticleImage_pending_commit_honors_expiry_boundary(t *testing.T) {
	image, created, expires := pendingImage(t)
	if err := image.Commit(expires); err != nil {
		t.Fatalf("到期瞬间应允许确认：%v", err)
	}
	late, _, lateExpires := pendingImage(t)
	if err := late.Commit(lateExpires.Add(time.Nanosecond)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("过期确认应失败：%v", err)
	}
	_ = created
}

func Test_ArticleImage_orphan_restore_requires_monotonic_grace_time(t *testing.T) {
	image, created, _ := pendingImage(t)
	if err := image.Commit(created.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	orphaned := created.Add(2 * time.Hour)
	if err := image.Orphan(orphaned); err != nil {
		t.Fatal(err)
	}
	if err := image.Commit(orphaned.Add(-time.Nanosecond)); !errors.Is(err, ErrTimeRegression) {
		t.Fatalf("孤儿时间倒退应失败：%v", err)
	}
	if err := image.Commit(orphaned.Add(24 * time.Hour)); err != nil {
		t.Fatalf("宽限边界应允许恢复：%v", err)
	}
}

func pendingImage(t *testing.T) (*ArticleImage, time.Time, time.Time) {
	t.Helper()
	id, _ := NewArticleImageID("abcdef0123456789abcdef0123456789")
	owner, _ := NewImageOwnerID("0123456789abcdef0123456789abcdef")
	key, _ := NewStorageKey("11111111111111111111111111111111.jpg")
	created := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	expires := created.Add(24 * time.Hour)
	image, err := NewArticleImage(ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: MediaTypeJPEG, ByteSize: 1, Width: 1, Height: 1, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, created, expires)
	if err != nil {
		t.Fatal(err)
	}
	return image, created, expires
}
