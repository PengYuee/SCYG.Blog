//go:build integration

package postgres

import (
	"context"
	"errors"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"testing"
	"time"
)

func Test_ArticleImageRepository_count_lock_blocks_concurrent_reference_insert(t *testing.T) {
	fixture := openRepositoryFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	articleID, typeID, tagID := ids(t, 1, 1, 1)
	seedTaxonomy(t, fixture, typeID, tagID)
	article := newArticle(t, fixture, articleID, typeID, []domain.TagID{tagID}, "Lock", "lock")
	image := pendingImage(t, fixture.clock.now)
	if err := fixture.uow.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		if err := tx.Articles().Save(txctx, article); err != nil {
			return err
		}
		return tx.ArticleImages().Save(txctx, image)
	}); err != nil {
		t.Fatal(err)
	}
	locked := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		firstDone <- fixture.uow.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
			count, err := tx.ArticleImages().CountReferencesForUpdate(txctx, image.Metadata().ID)
			if err != nil {
				return err
			}
			if count != 0 {
				return errors.New("初始引用数不为零")
			}
			close(locked)
			select {
			case <-release:
				return nil
			case <-txctx.Done():
				return txctx.Err()
			}
		})
	}()
	select {
	case <-locked:
	case <-ctx.Done():
		t.Fatal("首事务未取得图片锁")
	}
	go func() {
		secondDone <- fixture.uow.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
			return tx.ArticleImages().ReplaceArticleReferences(txctx, articleID, []domain.ArticleImageID{image.Metadata().ID}, fixture.clock.now)
		})
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("并发引用未被图片行锁阻塞: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	if err := fixture.uow.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		count, err := tx.ArticleImages().CountReferencesForUpdate(txctx, image.Metadata().ID)
		if err == nil && count != 1 {
			t.Fatalf("最终引用数=%d", count)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
