//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
)

func Test_Repository_TagArticle_composite_key_rejects_duplicate(t *testing.T) {
	fixture := openRepositoryFixture(t)
	articleID, typeID, tagID := ids(t, 1, 1, 1)
	seedTaxonomy(t, fixture, typeID, tagID)
	article := newArticle(t, fixture, articleID, typeID, []domain.TagID{tagID}, "Link", "link")
	if err := fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error { return tx.Articles().Save(ctx, article) }); err != nil {
		t.Fatal(err)
	}
	err := fixture.db.GORM().Create(&tagArticleModel{ArticleID: articleID.Int64(), TagID: tagID.Int64()}).Error
	if !database.IsUnique(database.TranslateError(err)) {
		t.Fatalf("expected composite unique error, got %v", err)
	}
}
