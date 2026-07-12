//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"gorm.io/gorm"
)

func Test_Repository_stale_delete_is_rejected(t *testing.T) {
	fixture := openRepositoryFixture(t)
	articleID, typeID, tagID := ids(t, 1, 1, 1)
	seedTaxonomy(t, fixture, typeID, tagID)
	article := newArticle(t, fixture, articleID, typeID, []domain.TagID{tagID}, "Delete", "delete")
	if err := fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error { return tx.Articles().Save(ctx, article) }); err != nil {
		t.Fatal(err)
	}
	loaded, err := fixture.uowFindArticle(articleID)
	if err != nil {
		t.Fatal(err)
	}
	if err = loaded.Delete(loaded.Version(), fixedClock{now: fixture.clock.now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err = fixture.db.GORM().Model(&articleModel{}).Where(`"Id"=?`, articleID.Int64()).Update("Version", gorm.Expr(`"Version"+1`)).Error; err != nil {
		t.Fatal(err)
	}
	err = fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error { return tx.Articles().Save(ctx, loaded) })
	if err == nil {
		t.Fatal("stale delete succeeded")
	}
	var deleted bool
	fixture.db.GORM().Table(`"Article"`).Select(`"IsDeleted"`).Where(`"Id"=?`, articleID.Int64()).Scan(&deleted)
	if deleted {
		t.Fatal("stale delete changed row")
	}
}
func (fixture repositoryFixture) uowFindArticle(id domain.ArticleID) (article *domain.Article, err error) {
	err = fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error {
		article, err = tx.Articles().Find(ctx, id)
		return err
	})
	return article, err
}
