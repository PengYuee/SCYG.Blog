//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func Test_ArticleImageRepository_persists_references_and_rolls_back(t *testing.T) {
	fixture := openRepositoryFixture(t)
	articleID, typeID, tagID := ids(t, 1, 1, 1)
	seedTaxonomy(t, fixture, typeID, tagID)
	article := newArticle(t, fixture, articleID, typeID, []domain.TagID{tagID}, "Image article", "image-article")
	image := pendingImage(t, fixture.clock.now)
	ctx := context.Background()

	err := fixture.uow.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		if saveErr := tx.Articles().Save(txctx, article); saveErr != nil {
			return saveErr
		}
		if saveErr := tx.ArticleImages().Save(txctx, image); saveErr != nil {
			return saveErr
		}
		return tx.ArticleImages().ReplaceArticleReferences(txctx, articleID, []domain.ArticleImageID{image.Metadata().ID}, fixture.clock.now)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.uow.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		found, findErr := tx.ArticleImages().FindByStorageKey(txctx, image.Metadata().StorageKey)
		if findErr != nil {
			return findErr
		}
		count, countErr := tx.ArticleImages().CountReferencesForUpdate(txctx, found.Metadata().ID)
		if countErr != nil {
			return countErr
		}
		if count != 1 {
			t.Fatalf("引用数=%d", count)
		}
		return errors.New("强制回滚")
	})
	if err == nil {
		t.Fatal("应返回回滚错误")
	}
	var count int64
	if queryErr := fixture.db.GORM().Model(&articleImageReferenceModel{}).Where("image_id = ?", image.Metadata().ID.String()).Count(&count).Error; queryErr != nil || count != 1 {
		t.Fatalf("回滚破坏引用: count=%d err=%v", count, queryErr)
	}
}

func Test_ArticleImageRepository_orders_cleanup_candidates(t *testing.T) {
	fixture := openRepositoryFixture(t)
	ctx := context.Background()
	base := fixture.clock.now
	first := pendingImage(t, base.Add(-2*time.Hour))
	second := pendingImageWithID(t, base.Add(-time.Hour), "11111111111111111111111111111111")
	if err := fixture.uow.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		if err := tx.ArticleImages().Save(txctx, second); err != nil {
			return err
		}
		return tx.ArticleImages().Save(txctx, first)
	}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.uow.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		candidates, err := tx.ArticleImages().ListExpiredPending(txctx, base.Add(25*time.Hour), 10)
		if err != nil {
			return err
		}
		if len(candidates) != 2 || candidates[0].Metadata().ID != first.Metadata().ID {
			t.Fatal("候选顺序错误")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func pendingImage(t *testing.T, created time.Time) *domain.ArticleImage {
	return pendingImageWithID(t, created, "0123456789abcdef0123456789abcdef")
}
func pendingImageWithID(t *testing.T, created time.Time, rawID string) *domain.ArticleImage {
	t.Helper()
	id, _ := domain.NewArticleImageID(rawID)
	owner, _ := domain.NewImageOwnerID("abcdef0123456789abcdef0123456789")
	key, _ := domain.NewStorageKey(rawID + ".jpg")
	image, err := domain.NewArticleImage(domain.ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: domain.MediaTypeJPEG, ByteSize: 10, Width: 2, Height: 2, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}, created, created.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return image
}

func Test_ArticleImageRepository_owner_status_and_shared_reference_diff(t *testing.T) {
	fixture := openRepositoryFixture(t)
	ctx := context.Background()
	articleOne, typeID, tagID := ids(t, 1, 1, 1)
	articleTwo, _, _ := ids(t, 2, 1, 1)
	seedTaxonomy(t, fixture, typeID, tagID)
	firstArticle := newArticle(t, fixture, articleOne, typeID, []domain.TagID{tagID}, "First", "first")
	secondArticle := newArticle(t, fixture, articleTwo, typeID, []domain.TagID{tagID}, "Second", "second")
	image := pendingImage(t, fixture.clock.now)
	if err := fixture.uow.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		if err := tx.Articles().Save(txctx, firstArticle); err != nil {
			return err
		}
		if err := tx.Articles().Save(txctx, secondArticle); err != nil {
			return err
		}
		if err := tx.ArticleImages().Save(txctx, image); err != nil {
			return err
		}
		if err := tx.ArticleImages().ReplaceArticleReferences(txctx, articleOne, []domain.ArticleImageID{image.Metadata().ID}, fixture.clock.now); err != nil {
			return err
		}
		return tx.ArticleImages().ReplaceArticleReferences(txctx, articleTwo, []domain.ArticleImageID{image.Metadata().ID}, fixture.clock.now)
	}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.uow.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		owner, err := tx.ArticleImages().FindOwner(txctx, image.Metadata().ID)
		if err != nil {
			return err
		}
		if owner != image.Metadata().OwnerID {
			t.Fatal("owner不一致")
		}
		found, err := tx.ArticleImages().FindByStorageKey(txctx, image.Metadata().StorageKey)
		if err != nil {
			return err
		}
		if err = found.Commit(fixture.clock.now.Add(time.Hour)); err != nil {
			return err
		}
		if err = tx.ArticleImages().Save(txctx, found); err != nil {
			return err
		}
		if err = tx.ArticleImages().ReplaceArticleReferences(txctx, articleOne, nil, fixture.clock.now); err != nil {
			return err
		}
		count, err := tx.ArticleImages().CountReferencesForUpdate(txctx, image.Metadata().ID)
		if err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("shared count=%d", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := fixture.db.GORM().Table("article_images").Select("status").Where("id = ?", image.Metadata().ID.String()).Scan(&status).Error; err != nil || status != "committed" {
		t.Fatalf("status=%s err=%v", status, err)
	}
}

func Test_ArticleImageRepository_orders_and_limits_orphan_candidates(t *testing.T) {
	fixture := openRepositoryFixture(t)
	ctx := context.Background()
	base := fixture.clock.now
	first := pendingImage(t, base.Add(-4*time.Hour))
	second := pendingImageWithID(t, base.Add(-3*time.Hour), "22222222222222222222222222222222")
	for _, image := range []*domain.ArticleImage{first, second} {
		if err := image.Commit(base.Add(-2 * time.Hour)); err != nil {
			t.Fatal(err)
		}
		if err := image.Orphan(base.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := fixture.uow.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		if err := tx.ArticleImages().Save(txctx, second); err != nil {
			return err
		}
		return tx.ArticleImages().Save(txctx, first)
	}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.uow.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		got, err := tx.ArticleImages().ListExpiredOrphaned(txctx, base.Add(24*time.Hour), 1)
		if err != nil {
			return err
		}
		if len(got) != 1 || got[0].Metadata().ID.String() != "0123456789abcdef0123456789abcdef" {
			t.Fatalf("orphan candidates=%v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
