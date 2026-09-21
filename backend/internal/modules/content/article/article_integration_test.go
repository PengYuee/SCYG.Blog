//go:build integration

package article

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	platformdatabase "github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	qadatabase "github.com/PengYuee/SCYG.Blog/backend/internal/qa/database"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

type articleIntegrationAllowAll struct{}

func (articleIntegrationAllowAll) Authorize(context.Context, content.Action, content.Resource) error {
	return nil
}

type articleIntegrationClock struct{ now time.Time }

func (clock articleIntegrationClock) Now() time.Time { return clock.now }

type articleIntegrationFixture struct {
	db      *platformdatabase.Database
	service *Service
	typeID  int64
	tagID   int64
}

func openArticleIntegrationFixture(t *testing.T) articleIntegrationFixture {
	t.Helper()
	configPath := os.Getenv("QA_CONFIG")
	if configPath == "" {
		t.Fatal("QA_CONFIG is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	isolated, err := qadatabase.New(ctx, configPath, "content_article_")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := isolated.Close(cleanupCtx); err != nil {
			t.Error(err)
		}
	})
	pool, err := sql.Open("pgx", isolated.DSN())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := migrations.New(pool, "")
	if err != nil {
		_ = pool.Close()
		t.Fatal(err)
	}
	if err = runner.Up(); err != nil {
		_ = runner.Close()
		_ = pool.Close()
		t.Fatal(err)
	}
	if err = runner.Close(); err != nil {
		_ = pool.Close()
		t.Fatal(err)
	}
	if err = pool.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := platformdatabase.New(ctx, platformdatabase.Options{DSN: isolated.DSN(), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)), MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.GORM().Exec(`TRUNCATE article_image_references, article_images, article_tags, articles, tags, article_types RESTART IDENTITY CASCADE`).Error; err != nil {
		t.Fatal(err)
	}
	var typeID, tagID int64
	if err := db.GORM().Raw(`INSERT INTO article_types (name) VALUES (?) RETURNING id`, "article-integration-type").Scan(&typeID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM().Raw(`INSERT INTO tags (name) VALUES (?) RETURNING id`, "article-integration-tag").Scan(&tagID).Error; err != nil {
		t.Fatal(err)
	}
	clock := articleIntegrationClock{now: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}
	service, err := New(db.GORM(), Dependencies{Clock: clock, Authorizer: articleIntegrationAllowAll{}})
	if err != nil {
		t.Fatal(err)
	}
	return articleIntegrationFixture{db: db, service: service, typeID: typeID, tagID: tagID}
}

func TestServicePostgresPreservesLifecycleQueriesAndStaleVersion(t *testing.T) {
	fixture := openArticleIntegrationFixture(t)
	ctx := context.Background()
	created, err := fixture.service.Create(ctx, Create{Status: ArticleCreationStatusDraft, ArticleTypeID: fixture.typeID, Title: "Integration article", Slug: "integration-article", Digest: "digest", Content: "draft body", TagIDs: []int64{fixture.tagID}})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID <= 0 || created.Version != 1 || created.Status != string(StatusDraft) {
		t.Fatalf("created article = %#v", created)
	}
	if _, err := fixture.service.Get(ctx, Get{ID: created.ID}); !isArticleCode(err, CodeNotFound) {
		t.Fatalf("draft public read error = %v, want %s", err, CodeNotFound)
	}

	published, err := fixture.service.Publish(ctx, Publish{ID: created.ID, Version: created.Version})
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != string(StatusPublished) || published.Version != 2 {
		t.Fatalf("published article = %#v", published)
	}
	page, err := fixture.service.List(ctx, List{Page: 1, PageSize: 10, ArticleTypeID: fixture.typeID, TagID: fixture.tagID, Query: "Integration", Sort: "title"})
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalItems != 1 || len(page.Items) != 1 || page.Items[0].ID != created.ID || len(page.Items[0].TagIDs) != 1 || page.Items[0].TagIDs[0] != fixture.tagID {
		t.Fatalf("public page = %#v", page)
	}

	revised, err := fixture.service.Revise(ctx, Revise{ID: created.ID, Version: published.Version, ArticleTypeID: fixture.typeID, Title: "Integration article revised", Slug: "integration-article-revised", Digest: "updated digest", Content: "updated body", TagIDs: []int64{fixture.tagID}})
	if err != nil {
		t.Fatal(err)
	}
	if revised.Version != 3 || revised.Title != "Integration article revised" {
		t.Fatalf("revised article = %#v", revised)
	}
	_, err = fixture.service.Revise(ctx, Revise{ID: created.ID, Version: published.Version, ArticleTypeID: fixture.typeID, Title: "stale", Slug: "stale", Digest: "stale", Content: "stale", TagIDs: []int64{fixture.tagID}})
	if !isArticleCode(err, CodeStale) {
		t.Fatalf("stale revise error = %v, want %s", err, CodeStale)
	}

	if err := fixture.service.Delete(ctx, Delete{ID: created.ID, Version: revised.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Get(ctx, Get{ID: created.ID}); !isArticleCode(err, CodeNotFound) {
		t.Fatalf("deleted public read error = %v, want %s", err, CodeNotFound)
	}
}

func isArticleCode(err error, want Code) bool {
	var failure *Error
	return errors.As(err, &failure) && failure.Code == want
}
