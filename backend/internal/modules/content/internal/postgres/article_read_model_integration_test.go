//go:build integration

package postgres

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"gorm.io/gorm"
)

func Test_Repository_projections_filter_sort_page_hide_nonpublic_and_bound_queries(t *testing.T) {
	fixture := openRepositoryFixture(t)
	publishedID, typeID, tagA := ids(t, 1, 1, 1)
	_, _, tagB := ids(t, 2, 1, 2)
	seedTaxonomy(t, fixture, typeID, tagA, tagB)
	published := newArticle(t, fixture, publishedID, typeID, []domain.TagID{tagA}, "Alpha", "alpha")
	draft := newArticle(t, fixture, mustArticleID(t, 2), typeID, []domain.TagID{tagB}, "Beta", "beta")
	if err := fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error {
		if err := tx.Articles().Save(ctx, published); err != nil {
			return err
		}
		return tx.Articles().Save(ctx, draft)
	}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error {
		loaded, err := tx.Articles().Find(ctx, publishedID)
		if err != nil {
			return err
		}
		if err = loaded.Publish(loaded.Version(), fixedClock{now: fixture.clock.now.Add(time.Minute)}); err != nil {
			return err
		}
		return tx.Articles().Save(ctx, loaded)
	}); err != nil {
		t.Fatal(err)
	}
	var queries atomic.Int64
	callbackName := "task9_query_count"
	counter := func(*gorm.DB) { queries.Add(1) }
	if err := fixture.db.GORM().Callback().Query().Before("gorm:query").Register(callbackName, counter); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.GORM().Callback().Row().Before("gorm:row").Register(callbackName, counter); err != nil {
		t.Fatal(err)
	}
	page, err := fixture.read.ListPublished(context.Background(), application.ArticleFilter{Page: 1, PageSize: 1, TagID: tagA, Query: "Alpha", Sort: "title"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.GORM().Callback().Query().Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.GORM().Callback().Row().Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != publishedID || page.TotalItems != 1 || queries.Load() != 3 {
		t.Fatalf("public projection mismatch items=%d total=%d queries=%d", len(page.Items), page.TotalItems, queries.Load())
	}
	admin, err := fixture.read.ListAll(context.Background(), application.ArticleFilter{Page: 1, PageSize: 10, Status: domain.StatusDraft, Sort: "oldest"})
	if err != nil {
		t.Fatal(err)
	}
	if len(admin.Items) != 1 || admin.Items[0].Status != domain.StatusDraft {
		t.Fatalf("admin filter mismatch: %#v", admin)
	}
	if _, err = fixture.read.FindPublished(context.Background(), draft.ID()); err == nil {
		t.Fatal("draft leaked through public projection")
	}
	if _, err = fixture.read.ListPublished(context.Background(), application.ArticleFilter{Sort: `title; DROP TABLE "Article"`}); err == nil {
		t.Fatal("unsafe sort accepted")
	}
}
