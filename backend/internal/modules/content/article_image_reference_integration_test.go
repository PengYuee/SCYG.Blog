//go:build integration

package content_test

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	contentpostgres "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/postgres"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/blobstorage"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	qadatabase "github.com/PengYuee/SCYG.Blog/backend/internal/qa/database"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

const integrationOwner = "abcdef0123456789abcdef0123456789"

type integrationAuthorizer struct{}

func (integrationAuthorizer) Authorize(context.Context, module.Action, module.Resource) error {
	return nil
}

type integrationImageContent struct{ *bytes.Reader }

func (content integrationImageContent) ReadArticleImage(buffer []byte) (int, error) {
	return content.Read(buffer)
}

type articleReferenceFixture struct {
	database *database.Database
	module   *module.Module
	root     string
	storage  *blobstorage.Filesystem
}

func openArticleReferenceFixture(t *testing.T) articleReferenceFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	isolated, err := qadatabase.New(ctx, "todo_seven_")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if closeErr := isolated.Close(cleanupCtx); closeErr != nil {
			t.Error(closeErr)
		}
	})
	migrationPool, err := sql.Open("pgx", isolated.DSN())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := migrations.New(migrationPool, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err = runner.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := database.New(ctx, database.Options{DSN: isolated.DSN(), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)), MaxOpenConns: 8, MaxIdleConns: 4, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	storage, err := blobstorage.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := storage.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		if closeErr := db.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	authorID, err := module.NewAuthorID(integrationOwner)
	if err != nil {
		t.Fatal(err)
	}
	service, err := contentpostgres.New(contentpostgres.Dependencies{Database: db, Authorizer: integrationAuthorizer{}, CurrentAuthor: module.NewFixedCurrentAuthorProvider(authorID), ImageFilesystem: storage, ImagePolicy: module.DefaultArticleImagePolicy()})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GORM().Exec(`INSERT INTO "ArticleType" ("Id","Name","Version","CreationTime","LastModificationTime","IsDeleted") VALUES (1,'类型',1,now(),now(),false); INSERT INTO "Tag" ("Id","Name","Version","CreationTime","LastModificationTime","IsDeleted") VALUES (1,'标签',1,now(),now(),false)`).Error; err != nil {
		t.Fatal(err)
	}

	return articleReferenceFixture{database: db, module: service, root: root, storage: storage}
}

func uploadReferenceImage(t *testing.T, service *module.Module) module.ArticleImageResult {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.White)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, picture, nil); err != nil {
		t.Fatal(err)
	}
	result, err := service.UploadArticleImage(context.Background(), module.UploadArticleImage{Content: integrationImageContent{Reader: bytes.NewReader(encoded.Bytes())}})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func createReferenceArticle(t *testing.T, service *module.Module, slug, markdown string) (module.ArticleResult, error) {
	t.Helper()
	return service.CreateArticle(context.Background(), module.CreateArticle{ArticleTypeID: 1, Title: "文章 " + slug, Slug: slug, Digest: "摘要", Content: markdown, TagIDs: []int64{1}})
}

func Test_CreateArticle_commits_deduplicated_pending_image_atomically(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	imageResult := uploadReferenceImage(t, fixture.module)
	markdown := "![一](" + imageResult.URL + ") ![重复](" + imageResult.URL + ")"

	// When
	article, err := createReferenceArticle(t, fixture.module, "deduplicated", markdown)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	var references, committed int64
	if queryErr := fixture.database.GORM().Table("article_image_references").Where("article_id = ?", article.ID).Count(&references).Error; queryErr != nil {
		t.Fatal(queryErr)
	}
	if queryErr := fixture.database.GORM().Table("article_images").Where("id = ? AND status = 'committed'", imageResult.ID).Count(&committed).Error; queryErr != nil {
		t.Fatal(queryErr)
	}
	if references != 1 || committed != 1 {
		t.Fatalf("references=%d committed=%d", references, committed)
	}
}

func Test_CreateArticle_rolls_back_when_managed_image_file_is_missing(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	imageResult := uploadReferenceImage(t, fixture.module)
	if err := os.Remove(filepath.Join(fixture.root, imageResult.StorageKey)); err != nil {
		t.Fatal(err)
	}

	// When
	_, err := createReferenceArticle(t, fixture.module, "missing-file", "![图]("+imageResult.URL+")")

	// Then
	if err == nil {
		t.Fatal("CreateArticle() error = nil")
	}
	var articles, pending int64
	_ = fixture.database.GORM().Table(`"Article"`).Count(&articles).Error
	_ = fixture.database.GORM().Table("article_images").Where("id = ? AND status = 'pending'", imageResult.ID).Count(&pending).Error
	if articles != 0 || pending != 1 {
		t.Fatalf("articles=%d pending=%d", articles, pending)
	}
}

func Test_CreateArticle_rolls_back_when_managed_image_file_exceeds_limit(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	imageResult := uploadReferenceImage(t, fixture.module)
	path := filepath.Join(fixture.root, imageResult.StorageKey)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, 5<<20+1), 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	_, err := createReferenceArticle(t, fixture.module, "oversized-file", "![图]("+imageResult.URL+")")

	// Then
	if err == nil {
		t.Fatal("CreateArticle() error = nil")
	}
	var articles, pending int64
	_ = fixture.database.GORM().Table(`"Article"`).Count(&articles).Error
	_ = fixture.database.GORM().Table("article_images").Where("id = ? AND status = 'pending'", imageResult.ID).Count(&pending).Error
	if articles != 0 || pending != 1 {
		t.Fatalf("articles=%d pending=%d", articles, pending)
	}
}

func Test_PatchArticle_removes_then_recovers_image_inside_grace(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	imageResult := uploadReferenceImage(t, fixture.module)
	created, err := createReferenceArticle(t, fixture.module, "recover-image", "![图]("+imageResult.URL+")")
	if err != nil {
		t.Fatal(err)
	}
	external := "![外部](https://example.com/image.jpg)"

	// When
	removed, err := fixture.module.PatchArticle(context.Background(), module.PatchArticle{ID: created.ID, Version: created.Version, Content: &external})
	if err != nil {
		t.Fatal(err)
	}
	recoveredMarkdown := "![图](" + imageResult.URL + ")"
	_, err = fixture.module.PatchArticle(context.Background(), module.PatchArticle{ID: removed.ID, Version: removed.Version, Content: &recoveredMarkdown})
	// Then
	if err != nil {
		t.Fatal(err)
	}
	var references, committed int64
	_ = fixture.database.GORM().Table("article_image_references").Where("article_id = ?", created.ID).Count(&references).Error
	_ = fixture.database.GORM().Table("article_images").Where("id = ? AND status = 'committed' AND orphaned_at IS NULL", imageResult.ID).Count(&committed).Error
	if references != 1 || committed != 1 {
		t.Fatalf("references=%d committed=%d", references, committed)
	}
}
