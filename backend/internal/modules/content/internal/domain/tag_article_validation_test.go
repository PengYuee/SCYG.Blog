package domain

import (
	"errors"
	"testing"
)

func Test_TagArticle_rejects_zero_identifiers(t *testing.T) {
	articleID, _ := NewArticleID(1)
	tagID, _ := NewTagID(1)
	for _, pair := range []struct {
		articleID ArticleID
		tagID     TagID
	}{{ArticleID{}, tagID}, {articleID, TagID{}}} {
		if _, err := NewTagArticle(pair.articleID, pair.tagID); !errors.Is(err, ErrInvalidValue) {
			t.Fatalf("expected invalid association, got %v", err)
		}
	}
}
