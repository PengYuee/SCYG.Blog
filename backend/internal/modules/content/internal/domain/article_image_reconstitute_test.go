package domain

import (
	"testing"
	"time"
)

func Test_ReconstituteArticleImage_accepts_valid_committed_state(t *testing.T) {
	created := time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC)
	state := validArticleImageState(t, created)
	state.Status, state.CommittedAt = ArticleImageStatusCommitted, created.Add(time.Hour)

	image, err := ReconstituteArticleImage(state)

	if err != nil {
		t.Fatal(err)
	}
	if image.Status() != ArticleImageStatusCommitted || image.Metadata().StorageKey.String() != state.Metadata.StorageKey.String() {
		t.Fatal("重建结果不一致")
	}
}

func Test_ReconstituteArticleImage_rejects_invalid_timestamp_combinations(t *testing.T) {
	created := time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC)
	tests := []ArticleImageState{
		func() ArticleImageState {
			value := validArticleImageState(t, created)
			value.Status = ArticleImageStatusCommitted
			return value
		}(),
		func() ArticleImageState {
			value := validArticleImageState(t, created)
			value.Status = ArticleImageStatusOrphaned
			value.CommittedAt = created.Add(time.Hour)
			return value
		}(),
		func() ArticleImageState {
			value := validArticleImageState(t, created)
			value.ExpiresAt = created
			return value
		}(),
	}
	for _, state := range tests {
		if _, err := ReconstituteArticleImage(state); err == nil {
			t.Fatal("非法持久化状态未被拒绝")
		}
	}
}

func validArticleImageState(t *testing.T, created time.Time) ArticleImageState {
	t.Helper()
	id, _ := NewArticleImageID("0123456789abcdef0123456789abcdef")
	owner, _ := NewImageOwnerID("abcdef0123456789abcdef0123456789")
	key, _ := NewStorageKey("0123456789abcdef0123456789abcdef.jpg")
	return ArticleImageState{Metadata: ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: MediaTypeJPEG, ByteSize: 10, Width: 2, Height: 2, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}, Status: ArticleImageStatusPending, CreatedAt: created, ExpiresAt: created.Add(24 * time.Hour)}
}

func Test_ReconstituteArticleImage_rejects_orphan_expiry_before_orphaned_at(t *testing.T) {
	created := time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC)
	state := validArticleImageState(t, created)
	state.Status = ArticleImageStatusOrphaned
	state.CommittedAt = created.Add(time.Hour)
	state.OrphanedAt = created.Add(2 * time.Hour)
	state.ExpiresAt = state.OrphanedAt.Add(-time.Second)
	if _, err := ReconstituteArticleImage(state); err == nil {
		t.Fatal("接受了早于孤儿时间的过期时间")
	}
}
