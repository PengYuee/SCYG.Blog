//go:build integration

package application_test

import (
	"bytes"
	"context"
	stdsql "database/sql"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	featureimage "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/image"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/blobstorage"
	platformdatabase "github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	qadatabase "github.com/PengYuee/SCYG.Blog/backend/internal/qa/database"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

type applicationAllowAll struct{}

func (applicationAllowAll) Authorize(context.Context, module.Action, module.Resource) error {
	return nil
}

type applicationClock struct{ now time.Time }

func (clock *applicationClock) Now() time.Time { return clock.now }

func applicationImageBytes(t *testing.T) []byte {
	t.Helper()
	source := image.NewRGBA(image.Rect(0, 0, 2, 2))
	source.Set(0, 0, color.White)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, nil); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

type applicationFixture struct {
	db       *platformdatabase.Database
	store    *blobstorage.Filesystem
	workflow *application.ArticleImages
	images   *featureimage.Service
	clock    *applicationClock
	typeID   int64
	tagID    int64
}

func newApplicationFixture(t *testing.T) applicationFixture {
	t.Helper()
	configPath := os.Getenv("QA_CONFIG")
	if configPath == "" {
		t.Fatal("QA_CONFIG is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	isolated, err := qadatabase.New(ctx, configPath, "article_images_")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		if err := isolated.Close(cleanupCtx); err != nil {
			t.Error(err)
		}
	})
	migrationDB, err := stdsql.Open("pgx", isolated.DSN())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := migrations.New(migrationDB, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := platformdatabase.New(ctx, platformdatabase.Options{DSN: isolated.DSN(), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)), MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, err := blobstorage.New(root)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_ = db.Close()
	})
	clock := &applicationClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	author, err := module.NewAuthorID("abcdef0123456789abcdef0123456789")
	if err != nil {
		t.Fatal(err)
	}
	authorizer := applicationAllowAll{}
	articleService, err := article.New(db.GORM(), article.Dependencies{Authorizer: authorizer, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	policy := featureimage.DefaultPolicy()
	imageService, err := featureimage.New(db.GORM(), featureimage.Dependencies{Authorizer: authorizer, CurrentAuthor: module.NewFixedCurrentAuthorProvider(author), Blob: featureimage.NewFilesystemBlob(store, policy), Clock: clock, Policy: policy})
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := application.NewArticleImages(application.Dependencies{DB: db.GORM(), Authorizer: authorizer, CurrentAuthor: module.NewFixedCurrentAuthorProvider(author), Clock: clock, Articles: articleService, Images: imageService})
	if err != nil {
		t.Fatal(err)
	}
	var typeID, tagID int64
	if err := db.GORM().Raw(`INSERT INTO article_types (name) VALUES (?) RETURNING id`, "application-test-type").Scan(&typeID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM().Raw(`INSERT INTO tags (name) VALUES (?) RETURNING id`, "application-test-tag").Scan(&tagID).Error; err != nil {
		t.Fatal(err)
	}
	return applicationFixture{db: db, store: store, workflow: workflow, images: imageService, clock: clock, typeID: typeID, tagID: tagID}
}

func uploadApplicationImage(t *testing.T, fixture applicationFixture) featureimage.Result {
	t.Helper()
	result, err := fixture.images.Upload(context.Background(), featureimage.Upload{Content: bytes.NewReader(applicationImageBytes(t))})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func createApplicationArticle(t *testing.T, fixture applicationFixture, title, content string) article.Result {
	t.Helper()
	result, err := fixture.workflow.Create(context.Background(), article.Create{Status: article.ArticleCreationStatusDraft, ArticleTypeID: fixture.typeID, Title: title, Slug: title, Digest: "digest", Content: content, TagIDs: []int64{fixture.tagID}})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestArticleImagesTransactionsAndReferenceLifecycle(t *testing.T) {
	fixture := newApplicationFixture(t)
	first := uploadApplicationImage(t, fixture)
	if err := fixture.db.GORM().Table("article_images").Where("id = ?", first.ID).Update("owner_id", "11111111111111111111111111111111").Error; err != nil {
		t.Fatal(err)
	}
	rolledBack, err := fixture.workflow.Create(context.Background(), article.Create{Status: article.ArticleCreationStatusDraft, ArticleTypeID: fixture.typeID, Title: "rolled-back", Slug: "rolled-back", Digest: "digest", Content: "![image](/media/article-images/" + first.StorageKey + ")", TagIDs: []int64{fixture.tagID}})
	if err == nil {
		t.Fatal("owner mismatch unexpectedly allowed")
	}
	if rolledBack.ID != 0 {
		t.Fatalf("rolled-back result exposed id %d", rolledBack.ID)
	}
	var articleCount int64
	if err := fixture.db.GORM().Table("articles").Where("title = ?", "rolled-back").Count(&articleCount).Error; err != nil {
		t.Fatal(err)
	}
	if articleCount != 0 {
		t.Fatalf("rolled-back article count=%d", articleCount)
	}

	second := uploadApplicationImage(t, fixture)
	third := uploadApplicationImage(t, fixture)
	firstArticle := createApplicationArticle(t, fixture, "first-article", "![image](/media/article-images/"+second.StorageKey+")")
	secondArticle := createApplicationArticle(t, fixture, "second-article", "![image](/media/article-images/"+second.StorageKey+")")
	patched, err := fixture.workflow.Patch(context.Background(), article.Patch{ID: firstArticle.ID, Version: firstArticle.Version, Content: new("![image](/media/article-images/" + third.StorageKey + ")")})
	if err != nil {
		t.Fatal(err)
	}
	if patched.Version != firstArticle.Version+1 {
		t.Fatalf("patched version=%d", patched.Version)
	}
	assertImageStatus(t, fixture, second.ID, "committed")
	assertImageStatus(t, fixture, third.ID, "committed")
	if _, err := fixture.workflow.Patch(context.Background(), article.Patch{ID: secondArticle.ID, Version: secondArticle.Version, Content: new("plain body")}); err != nil {
		t.Fatal(err)
	}
	assertImageStatus(t, fixture, second.ID, "orphaned")

	current, err := fixture.workflow.Patch(context.Background(), article.Patch{ID: secondArticle.ID, Version: secondArticle.Version + 1, Content: new("updated body")})
	if err != nil {
		t.Fatal(err)
	}
	staleContent := "![missing](/media/article-images/missing.jpg)"
	_, err = fixture.workflow.Patch(context.Background(), article.Patch{ID: current.ID, Version: secondArticle.Version + 1, Content: &staleContent})
	var appErr *application.Error
	if !errors.As(err, &appErr) || appErr.Code != "stale_version" {
		t.Fatalf("stale patch error=%v", err)
	}
}

func assertImageStatus(t *testing.T, fixture applicationFixture, id, want string) {
	t.Helper()
	var status string
	if err := fixture.db.GORM().Table("article_images").Where("id = ?", id).Pluck("status", &status).Error; err != nil {
		t.Fatal(err)
	}
	if status != want {
		t.Fatalf("image %s status=%q want %q", id, status, want)
	}
}
