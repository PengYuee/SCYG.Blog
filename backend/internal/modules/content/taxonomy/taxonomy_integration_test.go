//go:build integration

package taxonomy

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
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	qadatabase "github.com/PengYuee/SCYG.Blog/backend/internal/qa/database"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

type allowAll struct{}

func (allowAll) Authorize(context.Context, content.Action, content.Resource) error { return nil }

type postgresFixture struct {
	db      *database.Database
	service *Service
}

func openPostgresFixture(t *testing.T) postgresFixture {
	t.Helper()
	configPath := os.Getenv("QA_CONFIG")
	if configPath == "" {
		t.Fatal("QA_CONFIG is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	isolated, err := qadatabase.New(ctx, configPath, "content_taxonomy_")
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
		t.Fatal(err)
	}
	if err = runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err = runner.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := database.New(ctx, database.Options{DSN: isolated.DSN(), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)), MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.GORM().Exec(`TRUNCATE article_image_references, article_images, article_tags, articles, tags, article_types RESTART IDENTITY CASCADE`).Error; err != nil {
			t.Error(err)
		}
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.GORM().Exec(`TRUNCATE article_image_references, article_images, article_tags, articles, tags, article_types RESTART IDENTITY CASCADE`).Error; err != nil {
		t.Fatal(err)
	}
	service, err := New(db.GORM(), Dependencies{Clock: fixedClock{now: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}, Authorizer: allowAll{}})
	if err != nil {
		t.Fatal(err)
	}
	return postgresFixture{db: db, service: service}
}

func taxonomyFailure(t *testing.T, err error, want Code) {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != want {
		t.Fatalf("error = %v, want %s", err, want)
	}
}

func Test_TaxonomyService_Postgres_preservesConstraintsAndDeletionRules(t *testing.T) {
	fixture := openPostgresFixture(t)
	ctx := context.Background()
	typeResult, err := fixture.service.CreateArticleType(ctx, CreateArticleType{Name: "News", Meun: 7})
	if err != nil || typeResult.ID <= 0 || typeResult.Version != 1 {
		t.Fatalf("create article type = %#v, err=%v", typeResult, err)
	}
	_, err = fixture.service.CreateArticleType(ctx, CreateArticleType{Name: "News"})
	taxonomyFailure(t, err, CodeAlreadyExists)

	tagResult, err := fixture.service.CreateTag(ctx, CreateTag{Name: "Go"})
	if err != nil || tagResult.ID <= 0 || tagResult.Version != 1 {
		t.Fatalf("create tag = %#v, err=%v", tagResult, err)
	}
	_, err = fixture.service.PatchArticleType(ctx, PatchArticleType{ID: typeResult.ID, Version: 2, Name: new("Other")})
	taxonomyFailure(t, err, CodeStaleVersion)

	created := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	if err := fixture.db.GORM().Exec(`INSERT INTO articles (id, article_type_id, title, slug, digest, content, status, version, created_at, is_deleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, false)`, 1, typeResult.ID, "Title", "title", "digest", "content", 1, 1, created).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.GORM().Exec(`INSERT INTO article_tags (article_id, tag_id) VALUES (?, ?)`, 1, tagResult.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.GORM().Exec(`INSERT INTO article_tags (article_id, tag_id) VALUES (?, ?)`, 1, 999999).Error; err == nil {
		t.Fatal("invalid tag reference was accepted")
	} else {
		taxonomyFailure(t, translateDatabase(err), CodeFailedPrecondition)
	}
	if err := fixture.service.DeleteTag(ctx, DeleteTag{ID: tagResult.ID, Version: 1}); err == nil {
		t.Fatal("active article reference was allowed to delete tag")
	} else {
		taxonomyFailure(t, err, CodeFailedPrecondition)
	}
	archiveType, err := fixture.service.CreateArticleType(ctx, CreateArticleType{Name: "Archive"})
	if err != nil {
		t.Fatalf("create second article type: %v", err)
	}
	if page, err := fixture.service.ListArticleTypes(ctx, ListArticleTypes{Page: 1, PageSize: 1, Sort: "title"}); err != nil || page.TotalItems != 2 || page.TotalPages != 2 || len(page.Items) != 1 || page.Items[0].ID != archiveType.ID {
		t.Fatalf("list article types = %#v, err=%v", page, err)
	}
	if page, err := fixture.service.ListArticleTypes(ctx, ListArticleTypes{Page: 2, PageSize: 1, Sort: "title"}); err != nil || len(page.Items) != 1 || page.Items[0].ID != typeResult.ID {
		t.Fatalf("second article type page = %#v, err=%v", page, err)
	}
	rustTag, err := fixture.service.CreateTag(ctx, CreateTag{Name: "Rust"})
	if err != nil {
		t.Fatalf("create second tag: %v", err)
	}
	if page, err := fixture.service.ListTags(ctx, ListTags{Page: 1, PageSize: 1, Sort: "-title"}); err != nil || page.TotalItems != 2 || len(page.Items) != 1 || page.Items[0].ID != rustTag.ID {
		t.Fatalf("list tags = %#v, err=%v", page, err)
	}
	if err := fixture.service.DeleteArticleType(ctx, DeleteArticleType{ID: typeResult.ID, Version: 1}); err == nil {
		t.Fatal("active article reference was allowed to delete article type")
	} else {
		taxonomyFailure(t, err, CodeFailedPrecondition)
	}
	if err := fixture.db.GORM().Exec(`UPDATE articles SET is_deleted = true, deleted_at = ?, updated_at = ?, version = 2 WHERE id = 1`, created, created).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.DeleteArticleType(ctx, DeleteArticleType{ID: typeResult.ID, Version: 1}); err != nil {
		t.Fatalf("soft-deleted article reference blocked type deletion: %v", err)
	}
	if err := fixture.service.DeleteTag(ctx, DeleteTag{ID: tagResult.ID, Version: 1}); err != nil {
		t.Fatalf("soft-deleted article reference blocked tag deletion: %v", err)
	}
}
