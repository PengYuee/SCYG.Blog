//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func Test_Repository_CRUD_persists_reloads_updates_tags_and_soft_deletes(t *testing.T) {
	// Given
	fixture := openRepositoryFixture(t)
	articleID, typeID, tagA := ids(t, 1, 1, 1)
	_, _, tagB := ids(t, 2, 1, 2)
	seedTaxonomy(t, fixture, typeID, tagA, tagB)
	article := newArticle(t, fixture, articleID, typeID, []domain.TagID{tagA}, "First", "first")
	// When
	err := fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error { return tx.Articles().Save(ctx, article) })
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error {
		loaded, findErr := tx.Articles().Find(ctx, articleID)
		if findErr != nil {
			return findErr
		}
		title, _ := domain.NewTitle("Changed")
		slug, _ := domain.NewSlug("changed")
		digest, _ := domain.NewDigest("changed digest")
		body, _ := domain.NewContent("changed body")
		if reviseErr := loaded.Revise(loaded.Version(), domain.ArticleRevision{ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: body, TagIDs: []domain.TagID{tagB}}, fixedClock{now: fixture.clock.now.Add(time.Minute)}); reviseErr != nil {
			return reviseErr
		}
		return tx.Articles().Save(ctx, loaded)
	})
	if err != nil {
		t.Fatal(err)
	}
	// Then
	err = fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error {
		loaded, findErr := tx.Articles().Find(ctx, articleID)
		if findErr != nil {
			return findErr
		}
		if loaded.Title().String() != "Changed" || len(loaded.TagIDs()) != 1 || loaded.TagIDs()[0] != tagB {
			return errors.New("updated aggregate mismatch")
		}
		if deleteErr := loaded.Delete(loaded.Version(), fixedClock{now: fixture.clock.now.Add(2 * time.Minute)}); deleteErr != nil {
			return deleteErr
		}
		return tx.Articles().Save(ctx, loaded)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error {
		_, findErr := tx.Articles().Find(ctx, articleID)
		return findErr
	})
	assertNoAdapterCause(t, err)
	var appErr *content.ApplicationError
	if !errors.As(err, &appErr) || appErr.Code != content.CodeNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func Test_Repository_conflicts_stale_and_referenced_deletion_leave_rows_unchanged(t *testing.T) {
	fixture := openRepositoryFixture(t)
	articleID, typeID, tagID := ids(t, 1, 1, 1)
	seedTaxonomy(t, fixture, typeID, tagID)
	article := newArticle(t, fixture, articleID, typeID, []domain.TagID{tagID}, "Unique", "unique")
	if err := fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error { return tx.Articles().Save(ctx, article) }); err != nil {
		t.Fatal(err)
	}
	// Given two concurrent snapshots.
	var first, second *domain.Article
	if err := fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error {
		var err error
		first, err = tx.Articles().Find(ctx, articleID)
		if err != nil {
			return err
		}
		second, err = tx.Articles().Find(ctx, articleID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	revise := func(item *domain.Article, titleValue string, clock time.Time) error {
		title, _ := domain.NewTitle(titleValue)
		slug, _ := domain.NewSlug(titleValue)
		digest, _ := domain.NewDigest("digest")
		body, _ := domain.NewContent("body")
		return item.Revise(item.Version(), domain.ArticleRevision{ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: body, TagIDs: []domain.TagID{tagID}}, fixedClock{now: clock})
	}
	if err := revise(first, "winner", fixture.clock.now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := revise(second, "stale", fixture.clock.now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error { return tx.Articles().Save(ctx, first) }); err != nil {
		t.Fatal(err)
	}
	err := fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error { return tx.Articles().Save(ctx, second) })
	assertNoAdapterCause(t, err)
	var staleErr *content.ApplicationError
	if !errors.As(err, &staleErr) || staleErr.Code != content.CodeStaleVersion {
		t.Fatalf("expected stale, got %v", err)
	}
	// Referenced ArticleType deletion is rejected without changing its row.
	err = fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error {
		item, findErr := tx.ArticleTypes().Find(ctx, typeID)
		if findErr != nil {
			return findErr
		}
		if deleteErr := item.Delete(item.Version(), fixedClock{now: fixture.clock.now.Add(3 * time.Minute)}); deleteErr != nil {
			return deleteErr
		}
		return tx.ArticleTypes().Save(ctx, item)
	})
	if !errors.As(err, &staleErr) || staleErr.Code != content.CodeFailedPrecondition {
		t.Fatalf("expected protection, got %v", err)
	}
	var deleted bool
	if scanErr := fixture.db.GORM().Table(`"ArticleType"`).Select(`"IsDeleted"`).Where(`"Id"=?`, typeID.Int64()).Scan(&deleted).Error; scanErr != nil || deleted {
		t.Fatalf("article type changed: %v", scanErr)
	}
}

func Test_Repository_constraints_cancellation_and_transaction_rollback(t *testing.T) {
	fixture := openRepositoryFixture(t)
	articleID, typeID, tagID := ids(t, 1, 1, 1)
	seedTaxonomy(t, fixture, typeID, tagID)
	article := newArticle(t, fixture, articleID, typeID, []domain.TagID{tagID}, "Conflict", "conflict")
	if err := fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error { return tx.Articles().Save(ctx, article) }); err != nil {
		t.Fatal(err)
	}
	duplicate := newArticle(t, fixture, mustArticleID(t, 2), typeID, []domain.TagID{tagID}, "Conflict", "other")
	err := fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error { return tx.Articles().Save(ctx, duplicate) })
	assertNoAdapterCause(t, err)
	var appErr *content.ApplicationError
	if !errors.As(err, &appErr) || appErr.Code != content.CodeAlreadyExists {
		t.Fatalf("expected unique conflict, got %v", err)
	}
	slugDuplicate := newArticle(t, fixture, mustArticleID(t, 4), typeID, []domain.TagID{tagID}, "Other", "conflict")
	err = fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error {
		return tx.Articles().Save(ctx, slugDuplicate)
	})
	assertNoAdapterCause(t, err)
	if !errors.As(err, &appErr) || appErr.Code != content.CodeAlreadyExists {
		t.Fatalf("expected slug conflict, got %v", err)
	}
	missingType, _ := domain.NewArticleTypeID(999)
	missing := newArticle(t, fixture, mustArticleID(t, 3), missingType, []domain.TagID{tagID}, "Missing", "missing")
	err = fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error { return tx.Articles().Save(ctx, missing) })
	assertNoAdapterCause(t, err)
	if !errors.As(err, &appErr) || appErr.Code != content.CodeFailedPrecondition {
		t.Fatalf("expected foreign key, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = fixture.read.ListPublished(ctx, application.ArticleFilter{}); err == nil {
		t.Fatal("expected cancellation")
	}
	assertNoAdapterCause(t, err)
	missingTag, _ := domain.NewTagID(999)
	err = fixture.uow.Within(context.Background(), func(ctx context.Context, tx application.Transaction) error {
		loaded, findErr := tx.Articles().Find(ctx, articleID)
		if findErr != nil {
			return findErr
		}
		title, _ := domain.NewTitle("Rolledback")
		slug, _ := domain.NewSlug("rolledback")
		digest, _ := domain.NewDigest("digest")
		body, _ := domain.NewContent("body")
		if reviseErr := loaded.Revise(loaded.Version(), domain.ArticleRevision{ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: body, TagIDs: []domain.TagID{missingTag}}, fixedClock{now: fixture.clock.now.Add(time.Minute)}); reviseErr != nil {
			return reviseErr
		}
		return tx.Articles().Save(ctx, loaded)
	})
	if err == nil {
		t.Fatal("expected association rollback")
	}
	assertNoAdapterCause(t, err)
	var title string
	fixture.db.GORM().Table(`"Article"`).Select(`"Title"`).Where(`"Id"=?`, articleID.Int64()).Scan(&title)
	if title != "Conflict" {
		t.Fatalf("aggregate was not rolled back: %s", title)
	}
}
func mustArticleID(t *testing.T, value int64) domain.ArticleID {
	t.Helper()
	id, err := domain.NewArticleID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
