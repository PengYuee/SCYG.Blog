package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func Test_ArticleType_Rename_and_Delete_use_clock_and_version(t *testing.T) {
	name, _ := domain.NewName("Engineering")
	item := domain.NewArticleType(mustTypeID(t, 1), name, fixedClock{time.Unix(1, 0)})
	renamed, _ := domain.NewName("Architecture")
	clock := fixedClock{time.Unix(2, 0)}
	if err := item.Rename(item.Version(), renamed, clock); err != nil {
		t.Fatal(err)
	}
	if item.Name() != renamed || item.Version().Uint64() != 2 {
		t.Fatal("rename mismatch")
	}
	if err := item.Delete(item.Version(), clock); err != nil {
		t.Fatal(err)
	}
	if item.DeletedAt() != clock.now.UTC() || item.Version().Uint64() != 3 {
		t.Fatal("delete mismatch")
	}
}

func Test_Tag_Rename_rejects_stale_version_without_mutation(t *testing.T) {
	name, _ := domain.NewName("Go")
	tag := domain.NewTag(mustTagID(t, 1), name, fixedClock{time.Unix(1, 0)})
	stale, _ := domain.NewVersion(2)
	renamed, _ := domain.NewName("Golang")
	err := tag.Rename(stale, renamed, fixedClock{time.Unix(2, 0)})
	if !errors.Is(err, domain.ErrStaleVersion) || tag.Name() != name || tag.Version().Uint64() != 1 {
		t.Fatal("stale rename was not atomic")
	}
}

func Test_Tag_name_rejects_blank_input(t *testing.T) {
	_, err := domain.NewName(" ")
	if !errors.Is(err, domain.ErrInvalidValue) {
		t.Fatalf("expected invalid name, got %v", err)
	}
}
