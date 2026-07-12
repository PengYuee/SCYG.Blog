//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	qadatabase "github.com/PengYuee/SCYG.Blog/backend/internal/qa/database"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

type repositoryFixture struct {
	db    *database.Database
	uow   *UnitOfWork
	read  *ReadModel
	clock fixedClock
}

func openRepositoryFixture(t *testing.T) repositoryFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	isolated, err := qadatabase.New(ctx, "content_postgres_")
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
	db, err := database.New(ctx, database.Options{DSN: isolated.DSN(), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)), MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	platformUOW, err := database.NewUnitOfWork(db)
	if err != nil {
		t.Fatal(err)
	}
	uow, err := NewUnitOfWork(platformUOW)
	if err != nil {
		t.Fatal(err)
	}
	read, err := NewReadModel(db.GORM())
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GORM().Exec(`TRUNCATE article_image_references, article_images, "TagArticle", "Article", "Tag", "ArticleType" RESTART IDENTITY CASCADE`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cleanErr := db.GORM().Exec(`TRUNCATE article_image_references, article_images, "TagArticle", "Article", "Tag", "ArticleType" RESTART IDENTITY CASCADE`).Error; cleanErr != nil {
			t.Error(cleanErr)
		}
		for _, table := range []string{"article_image_references", "article_images"} {
			var count int64
			if countErr := db.GORM().Table(table).Count(&count).Error; countErr != nil || count != 0 {
				t.Errorf("清理后 %s count=%d err=%v", table, count, countErr)
			}
		}
		sqlDB, sqlErr := db.GORM().DB()
		if sqlErr != nil {
			t.Error(sqlErr)
			return
		}
		if closeErr := db.Close(); closeErr != nil {
			t.Error(closeErr)
			return
		}
		if pingErr := sqlDB.Ping(); pingErr == nil {
			t.Error("数据库连接关闭后仍可 Ping")
		}
	})
	return repositoryFixture{db: db, uow: uow, read: read, clock: fixedClock{now: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}}
}
func ids(t *testing.T, article, articleType, tag int64) (domain.ArticleID, domain.ArticleTypeID, domain.TagID) {
	t.Helper()
	articleID, err := domain.NewArticleID(article)
	if err != nil {
		t.Fatal(err)
	}
	typeID, err := domain.NewArticleTypeID(articleType)
	if err != nil {
		t.Fatal(err)
	}
	tagID, err := domain.NewTagID(tag)
	if err != nil {
		t.Fatal(err)
	}
	return articleID, typeID, tagID
}
func seedTaxonomy(t *testing.T, fixture repositoryFixture, typeID domain.ArticleTypeID, tagIDs ...domain.TagID) {
	t.Helper()
	typeName, _ := domain.NewName("Type")
	articleType, err := domain.NewArticleType(typeID, typeName, fixture.clock)
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error {
		if saveErr := tx.ArticleTypes().Save(ctx, articleType); saveErr != nil {
			return saveErr
		}
		for index, id := range tagIDs {
			name, _ := domain.NewName("Tag" + string(rune('A'+index)))
			tag, createErr := domain.NewTag(id, name, fixture.clock)
			if createErr != nil {
				return createErr
			}
			if saveErr := tx.Tags().Save(ctx, tag); saveErr != nil {
				return saveErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func newArticle(t *testing.T, fixture repositoryFixture, articleID domain.ArticleID, typeID domain.ArticleTypeID, tagIDs []domain.TagID, titleValue, slugValue string) *domain.Article {
	t.Helper()
	title, _ := domain.NewTitle(titleValue)
	slug, _ := domain.NewSlug(slugValue)
	digest, _ := domain.NewDigest("digest " + titleValue)
	body, _ := domain.NewContent("content")
	article, err := domain.NewArticle(domain.ArticleDraft{ID: articleID, ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: body, TagIDs: tagIDs}, fixture.clock)
	if err != nil {
		t.Fatal(err)
	}
	return article
}
