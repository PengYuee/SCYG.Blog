package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

func Test_Article_Revise_rejects_archived_article(t *testing.T) {
	article := newPublishedArticle(t)
	clock := fixedClock{time.Date(2026, 7, 11, 3, 0, 0, 0, time.UTC)}
	if err := article.Archive(article.Version(), clock); err != nil {
		t.Fatalf("archive: %v", err)
	}
	version := article.Version()
	before := article.Title()
	err := article.Revise(version, validRevision(t), clock)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("expected invalid transition, got %v", err)
	}
	if article.Version() != version || article.Title() != before {
		t.Fatal("failed revision changed aggregate")
	}
}

func Test_Article_lifecycle_increments_version_once_per_mutation(t *testing.T) {
	article := newArticle(t)
	revisedAt := fixedClock{time.Date(2026, 7, 11, 2, 0, 0, 0, time.UTC)}
	if err := article.Revise(article.Version(), validRevision(t), revisedAt); err != nil {
		t.Fatalf("revise: %v", err)
	}
	if article.Version().Uint64() != 2 || !article.ModifiedAt().Equal(revisedAt.now) {
		t.Fatal("revision version or clock mismatch")
	}
	if err := article.Publish(article.Version(), revisedAt); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if article.Status() != domain.StatusPublished || article.Version().Uint64() != 3 {
		t.Fatal("publish transition mismatch")
	}
	if err := article.Archive(article.Version(), revisedAt); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if article.Status() != domain.StatusArchived || article.Version().Uint64() != 4 {
		t.Fatal("archive transition mismatch")
	}
}

func Test_Article_Revise_rejects_stale_version_atomically(t *testing.T) {
	article := newArticle(t)
	stale, _ := domain.NewVersion(9)
	before := article.Title()
	err := article.Revise(stale, validRevision(t), fixedClock{time.Now()})
	var conflict *domain.VersionConflict
	if !errors.As(err, &conflict) || !errors.Is(err, domain.ErrStaleVersion) {
		t.Fatalf("expected typed stale conflict, got %v", err)
	}
	if article.Title() != before || article.Version().Uint64() != 1 {
		t.Fatal("stale revision changed aggregate")
	}
}

func Test_Article_NewArticle_rejects_duplicate_tags(t *testing.T) {
	draft := validDraft(t)
	draft.TagIDs = []domain.TagID{mustTagID(t, 1), mustTagID(t, 1)}
	_, err := domain.NewArticle(draft, fixedClock{time.Now()})
	if !errors.Is(err, domain.ErrDuplicateTag) {
		t.Fatalf("expected duplicate tag, got %v", err)
	}
}

func Test_Article_TagIDs_returns_stable_defensive_copy(t *testing.T) {
	article := newArticle(t)
	tags := article.TagIDs()
	tags[0] = mustTagID(t, 99)
	if article.TagIDs()[0].Int64() != 1 || article.TagIDs()[1].Int64() != 2 {
		t.Fatal("tag order or defensive copy violated")
	}
}

func newArticle(t *testing.T) *domain.Article {
	t.Helper()
	article, err := domain.NewArticle(validDraft(t), fixedClock{time.Date(2026, 7, 11, 1, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new article: %v", err)
	}
	return article
}
func newPublishedArticle(t *testing.T) *domain.Article {
	t.Helper()
	article := newArticle(t)
	if err := article.Publish(article.Version(), fixedClock{time.Now()}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return article
}
func validDraft(t *testing.T) domain.ArticleDraft {
	t.Helper()
	return domain.ArticleDraft{ID: mustArticleID(t, 1), ArticleTypeID: mustTypeID(t, 1), Title: mustTitle(t, "Original"), Slug: mustSlug(t, "original"), Digest: mustDigest(t, "Digest"), Content: mustContent(t, "Body"), TagIDs: []domain.TagID{mustTagID(t, 1), mustTagID(t, 2)}}
}
func validRevision(t *testing.T) domain.ArticleRevision {
	t.Helper()
	return domain.ArticleRevision{ArticleTypeID: mustTypeID(t, 2), Title: mustTitle(t, "Revised"), Slug: mustSlug(t, "revised"), Digest: mustDigest(t, "New digest"), Content: mustContent(t, "New body"), TagIDs: []domain.TagID{mustTagID(t, 2), mustTagID(t, 3)}}
}
func mustArticleID(t *testing.T, raw int64) domain.ArticleID {
	t.Helper()
	value, err := domain.NewArticleID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func mustTypeID(t *testing.T, raw int64) domain.ArticleTypeID {
	t.Helper()
	value, err := domain.NewArticleTypeID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func mustTagID(t *testing.T, raw int64) domain.TagID {
	t.Helper()
	value, err := domain.NewTagID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func mustTitle(t *testing.T, raw string) domain.Title {
	t.Helper()
	value, err := domain.NewTitle(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func mustSlug(t *testing.T, raw string) domain.Slug {
	t.Helper()
	value, err := domain.NewSlug(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func mustDigest(t *testing.T, raw string) domain.Digest {
	t.Helper()
	value, err := domain.NewDigest(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func mustContent(t *testing.T, raw string) domain.Content {
	t.Helper()
	value, err := domain.NewContent(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
