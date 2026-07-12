package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func Test_Tag_Rename_rejects_stale_version_without_mutation(t *testing.T) {
	name, _ := domain.NewName("Go")
	tag, err := domain.NewTag(mustTagID(t, 1), name, fixedClock{time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	stale, _ := domain.NewVersion(2)
	renamed, _ := domain.NewName("Golang")
	err = tag.Rename(stale, renamed, fixedClock{time.Unix(2, 0)})
	if !errors.Is(err, domain.ErrStaleVersion) || tag.Name() != name || tag.Version().Uint64() != 1 {
		t.Fatal("stale rename was not atomic")
	}
}

func Test_Tag_Delete_is_terminal(t *testing.T) {
	name, _ := domain.NewName("Go")
	tag, err := domain.NewTag(mustTagID(t, 1), name, fixedClock{time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if err := tag.Delete(tag.Version(), fixedClock{time.Unix(2, 0)}); err != nil {
		t.Fatal(err)
	}
	version := tag.Version()
	if err := tag.Rename(version, name, fixedClock{time.Unix(3, 0)}); !errors.Is(err, domain.ErrDeleted) {
		t.Fatalf("rename deleted tag: %v", err)
	}
	if err := tag.Delete(version, fixedClock{time.Unix(3, 0)}); !errors.Is(err, domain.ErrDeleted) {
		t.Fatalf("repeat delete tag: %v", err)
	}
}

func Test_Tag_name_rejects_blank_input(t *testing.T) {
	_, err := domain.NewName(" ")
	if !errors.Is(err, domain.ErrInvalidValue) {
		t.Fatalf("expected invalid name, got %v", err)
	}
}
