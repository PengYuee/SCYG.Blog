package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

type countingClock struct {
	now   time.Time
	calls int
}

func (clock *countingClock) Now() time.Time { clock.calls++; return clock.now }

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

func Test_Article_Revise_rejects_no_change_without_clock_or_version_change(t *testing.T) {
	article := internalArticle(t)
	clock := &countingClock{now: article.ModifiedAt().Add(time.Hour)}
	revision := ArticleRevision{ArticleTypeID: article.ArticleTypeID(), Title: article.Title(), Slug: article.Slug(), Digest: article.Digest(), Content: article.Content(), TagIDs: article.TagIDs()}
	version := article.Version()
	err := article.Revise(version, revision, clock)
	if !errors.Is(err, ErrNoChange) || clock.calls != 0 || article.Version() != version {
		t.Fatalf("no-op was not rejected atomically: %v", err)
	}
}

func Test_Article_mutations_reject_version_exhaustion_atomically(t *testing.T) {
	article := internalArticle(t)
	article.version = Version{math.MaxUint64}
	before := articleSnapshotOf(article)
	err := article.Revise(article.version, internalRevision(t), packageClock{article.ModifiedAt().Add(time.Hour)})
	if !errors.Is(err, ErrVersionExhausted) || articleSnapshotOf(article) != before {
		t.Fatalf("overflow mutation was not atomic: %v", err)
	}
}

func Test_Article_mutations_reject_backward_clock_atomically(t *testing.T) {
	article := internalArticle(t)
	before := articleSnapshotOf(article)
	err := article.Revise(article.Version(), internalRevision(t), packageClock{article.ModifiedAt().Add(-time.Second)})
	if !errors.Is(err, ErrTimeRegression) || articleSnapshotOf(article) != before {
		t.Fatalf("backward clock changed article: %v", err)
	}
}

func Test_Article_Delete_and_Archive_are_terminal(t *testing.T) {
	deleted := internalArticle(t)
	clock := packageClock{deleted.ModifiedAt().Add(time.Hour)}
	if err := deleted.Delete(deleted.Version(), clock); err != nil {
		t.Fatal(err)
	}
	version := deleted.Version()
	if err := deleted.Delete(version, clock); !errors.Is(err, ErrDeleted) {
		t.Fatalf("repeat delete: %v", err)
	}
	if err := deleted.Publish(version, clock); !errors.Is(err, ErrDeleted) {
		t.Fatalf("publish deleted: %v", err)
	}
	archived := internalArticle(t)
	if err := archived.Publish(archived.Version(), clock); err != nil {
		t.Fatal(err)
	}
	if err := archived.Archive(archived.Version(), clock); err != nil {
		t.Fatal(err)
	}
	archivedVersion := archived.Version()
	if err := archived.Archive(archivedVersion, clock); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("repeat archive: %v", err)
	}
	if err := archived.Delete(archivedVersion, clock); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("delete archived: %v", err)
	}
}

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

type articleSnapshot struct {
	articleTypeID ArticleTypeID
	title         Title
	slug          Slug
	digest        Digest
	content       Content
	status        Status
	tags          string
	version       Version
	modifiedAt    time.Time
	deletedAt     time.Time
}

func articleSnapshotOf(article *Article) articleSnapshot {
	return articleSnapshot{article.articleTypeID, article.title, article.slug, article.digest, article.content, article.status, tagKey(article.tagIDs), article.version, article.modifiedAt, article.deletedAt}
}
func tagKey(tags []TagID) string {
	result := ""
	for _, tag := range tags {
		result += string(rune(tag.Int64()))
	}
	return result
}
func internalArticle(t *testing.T) *Article {
	t.Helper()
	article, err := NewArticle(internalDraft(t), packageClock{time.Unix(10, 0)})
	if err != nil {
		t.Fatal(err)
	}
	return article
}
func internalDraft(t *testing.T) ArticleDraft {
	t.Helper()
	id, _ := NewArticleID(1)
	typeID, _ := NewArticleTypeID(1)
	title, _ := NewTitle("Title")
	slug, _ := NewSlug("title")
	digest, _ := NewDigest("Digest")
	content, _ := NewContent("Body")
	tag, _ := NewTagID(1)
	return ArticleDraft{ID: id, ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: content, TagIDs: []TagID{tag}}
}
func internalRevision(t *testing.T) ArticleRevision {
	t.Helper()
	typeID, _ := NewArticleTypeID(2)
	title, _ := NewTitle("Changed")
	slug, _ := NewSlug("changed")
	digest, _ := NewDigest("Changed digest")
	content, _ := NewContent("Changed body")
	tag, _ := NewTagID(2)
	return ArticleRevision{ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: content, TagIDs: []TagID{tag}}
}

func Test_Article_all_mutations_preflight_version_exhaustion(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(*Article)
		mutate  func(*Article, Clock) error
	}{
		{"revise", func(*Article) {}, func(article *Article, clock Clock) error {
			return article.Revise(article.version, internalRevision(t), clock)
		}},
		{"publish", func(*Article) {}, func(article *Article, clock Clock) error { return article.Publish(article.version, clock) }},
		{"archive", func(article *Article) { article.status = StatusPublished }, func(article *Article, clock Clock) error { return article.Archive(article.version, clock) }},
		{"delete", func(*Article) {}, func(article *Article, clock Clock) error { return article.Delete(article.version, clock) }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			article := internalArticle(t)
			testCase.prepare(article)
			article.version = Version{math.MaxUint64}
			before := articleSnapshotOf(article)
			err := testCase.mutate(article, packageClock{article.modifiedAt.Add(time.Hour)})
			if !errors.Is(err, ErrVersionExhausted) || articleSnapshotOf(article) != before {
				t.Fatalf("mutation was not atomic: %v", err)
			}
		})
	}
}

func Test_Article_Revise_rejects_forged_zero_values_atomically(t *testing.T) {
	base := internalRevision(t)
	cases := []struct {
		name   string
		mutate func(*ArticleRevision)
	}{
		{"article type", func(value *ArticleRevision) { value.ArticleTypeID = ArticleTypeID{} }},
		{"title", func(value *ArticleRevision) { value.Title = Title{} }},
		{"slug", func(value *ArticleRevision) { value.Slug = Slug{} }},
		{"digest", func(value *ArticleRevision) { value.Digest = Digest{} }},
		{"content", func(value *ArticleRevision) { value.Content = Content{} }},
		{"tag", func(value *ArticleRevision) { value.TagIDs = []TagID{{}} }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			article := internalArticle(t)
			before := articleSnapshotOf(article)
			revision := base
			testCase.mutate(&revision)
			err := article.Revise(article.version, revision, packageClock{article.modifiedAt.Add(time.Hour)})
			if err == nil || articleSnapshotOf(article) != before {
				t.Fatalf("forged value changed article: %v", err)
			}
		})
	}
}

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
