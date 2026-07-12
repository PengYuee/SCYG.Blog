package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func Test_ArticleType_and_Tag_reject_invalid_boundaries_and_overflow(t *testing.T) {
	name, _ := NewName("Name")
	now := packageClock{time.Unix(1, 0)}
	if _, err := NewArticleType(ArticleTypeID{}, name, now); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("zero type id: %v", err)
	}
	if _, err := NewTag(TagID{}, name, now); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("zero tag id: %v", err)
	}
	typeID, _ := NewArticleTypeID(1)
	item, _ := NewArticleType(typeID, name, now)
	item.version = Version{math.MaxUint64}
	renamed, _ := NewName("Renamed")
	beforeName, beforeTime := item.name, item.modifiedAt
	if err := item.Rename(item.version, renamed, packageClock{time.Unix(2, 0)}); !errors.Is(err, ErrVersionExhausted) || item.name != beforeName || item.modifiedAt != beforeTime {
		t.Fatalf("taxonomy overflow was not atomic: %v", err)
	}
}
