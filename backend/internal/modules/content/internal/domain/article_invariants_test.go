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

func Test_Article_Publish_rejects_missing_content_atomically(t *testing.T) {
	id, _ := NewArticleID(1)
	typeID, _ := NewArticleTypeID(1)
	title, _ := NewTitle("Title")
	slug, _ := NewSlug("title")
	digest, _ := NewDigest("Digest")
	tag, _ := NewTagID(1)
	article, err := NewArticle(ArticleDraft{ID: id, ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, TagIDs: []TagID{tag}}, packageClock{time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	version := article.Version()
	err = article.Publish(version, packageClock{time.Unix(2, 0)})
	if !errors.Is(err, ErrContentRequired) || article.Status() != StatusDraft || article.Version() != version {
		t.Fatalf("publish failure not atomic: %v", err)
	}
}
