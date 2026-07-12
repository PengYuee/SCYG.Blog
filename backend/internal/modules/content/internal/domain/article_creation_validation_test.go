package domain

import (
	"errors"
	"testing"
	"time"
)

type packageClock struct{ now time.Time }

func (clock packageClock) Now() time.Time { return clock.now }

func Test_Article_NewArticle_rejects_missing_article_type(t *testing.T) {
	id, _ := NewArticleID(1)
	title, _ := NewTitle("Title")
	slug, _ := NewSlug("title")
	digest, _ := NewDigest("Digest")
	body, _ := NewContent("Body")
	tag, _ := NewTagID(1)
	_, err := NewArticle(ArticleDraft{ID: id, Title: title, Slug: slug, Digest: digest, Content: body, TagIDs: []TagID{tag}}, packageClock{time.Unix(1, 0)})
	if !errors.Is(err, ErrArticleTypeRequired) {
		t.Fatalf("expected article type required, got %v", err)
	}
}

func Test_Article_NewArticle_rejects_missing_content(t *testing.T) {
	id, _ := NewArticleID(1)
	typeID, _ := NewArticleTypeID(1)
	title, _ := NewTitle("Title")
	slug, _ := NewSlug("title")
	digest, _ := NewDigest("Digest")
	tag, _ := NewTagID(1)
	_, err := NewArticle(ArticleDraft{ID: id, ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, TagIDs: []TagID{tag}}, packageClock{time.Unix(1, 0)})
	if !errors.Is(err, ErrContentRequired) {
		t.Fatalf("expected content required, got %v", err)
	}
}

func Test_Article_NewArticle_rejects_forged_zero_values_and_invalid_clock(t *testing.T) {
	valid := internalDraft(t)
	cases := []struct {
		name   string
		mutate func(*ArticleDraft)
		clock  Clock
	}{
		{"article id", func(value *ArticleDraft) { value.ID = ArticleID{} }, packageClock{time.Unix(1, 0)}},
		{"article type id", func(value *ArticleDraft) { value.ArticleTypeID = ArticleTypeID{} }, packageClock{time.Unix(1, 0)}},
		{"title", func(value *ArticleDraft) { value.Title = Title{} }, packageClock{time.Unix(1, 0)}},
		{"slug", func(value *ArticleDraft) { value.Slug = Slug{} }, packageClock{time.Unix(1, 0)}},
		{"digest", func(value *ArticleDraft) { value.Digest = Digest{} }, packageClock{time.Unix(1, 0)}},
		{"content", func(value *ArticleDraft) { value.Content = Content{} }, packageClock{time.Unix(1, 0)}},
		{"zero clock", func(*ArticleDraft) {}, packageClock{}},
		{"nil clock", func(*ArticleDraft) {}, nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			draft := valid
			testCase.mutate(&draft)
			_, err := NewArticle(draft, testCase.clock)
			if err == nil {
				t.Fatal("expected boundary rejection")
			}
		})
	}
}
