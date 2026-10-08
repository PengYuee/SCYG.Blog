//go:build integration

package application_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/operation"
)

const operationUser = "abcdef0123456789abcdef0123456789"

type mutablePolicy struct{ denied atomic.Bool }

func (policy *mutablePolicy) Authorize(context.Context, module.Action, module.Resource) error {
	if policy.denied.Load() {
		return module.ErrPermissionDenied
	}
	return nil
}

func operationFixture(t *testing.T) (applicationFixture, *application.ArticleOperations, *article.Service, *operation.Service, *mutablePolicy) {
	t.Helper()
	fixture := newApplicationFixture(t)
	policy := &mutablePolicy{}
	articles, err := article.New(fixture.db.GORM(), article.Dependencies{Clock: fixture.clock, Authorizer: policy})
	if err != nil {
		t.Fatal(err)
	}
	author, err := module.NewAuthorID(operationUser)
	if err != nil {
		t.Fatal(err)
	}
	images, err := application.NewArticleImages(application.Dependencies{DB: fixture.db.GORM(), Clock: fixture.clock, Authorizer: policy, CurrentAuthor: module.NewFixedCurrentAuthorProvider(author), Articles: articles, Images: fixture.images})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := operation.New(fixture.db.GORM())
	if err != nil {
		t.Fatal(err)
	}
	operations, err := application.NewArticleOperations(application.OperationDependencies{DB: fixture.db.GORM(), Articles: articles, ArticleImages: images, Operations: ledger})
	if err != nil {
		t.Fatal(err)
	}
	return fixture, operations, articles, ledger, policy
}

func operationCreate(fixture applicationFixture, title string) article.Create {
	return article.Create{Status: article.ArticleCreationStatusDraft, ArticleTypeID: fixture.typeID, Title: title, Slug: title, Digest: "digest", Content: "body", TagIDs: []int64{fixture.tagID}}
}

func TestArticleOperationsConcurrentSuccessAndAdvancedReplay(t *testing.T) {
	fixture, operations, _, _, _ := operationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	key := "10000000-0000-4000-8000-000000000001"
	const calls = 8
	results := make([]article.Result, calls)
	failures := make([]error, calls)
	start := make(chan struct{})
	var group sync.WaitGroup
	for i := range calls {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results[i], failures[i] = operations.Create(ctx, key, operationCreate(fixture, "concurrent"))
		}()
	}
	close(start)
	group.Wait()
	for i := range calls {
		if failures[i] != nil {
			t.Fatal(failures[i])
		}
		if results[i].ID != results[0].ID {
			t.Fatalf("duplicate article: %+v", results)
		}
	}
	var count int64
	if err := fixture.db.GORM().Table("articles").Where("title = ?", "concurrent").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("writes=%d", count)
	}
	next, err := operations.Patch(ctx, "10000000-0000-4000-8000-000000000002", article.Patch{ID: results[0].ID, Version: results[0].Version, Title: new("advanced")})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := operations.Publish(ctx, key, article.Publish{ID: -1, Version: 0})
	if err != nil || replay.ID != next.ID || replay.Version != next.Version || replay.Title != "advanced" {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
}

func TestArticleOperationsRetryPresenceExpiryAndSingleConnection(t *testing.T) {
	fixture, operations, _, ledger, _ := operationFixture(t)
	pool, err := fixture.db.GORM().DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	key := "20000000-0000-4000-8000-000000000001"
	invalid := operationCreate(fixture, "retry")
	invalid.Title = ""
	if _, err := operations.Create(ctx, key, invalid); err == nil {
		t.Fatal("invalid write succeeded")
	}
	first, err := operations.Create(ctx, key, operationCreate(fixture, "retry"))
	if err != nil {
		t.Fatal(err)
	}
	patched, err := operations.Patch(ctx, "20000000-0000-4000-8000-000000000002", article.Patch{ID: first.ID, Version: first.Version, Title: new("retained")})
	if err != nil || len(patched.TagIDs) != 1 {
		t.Fatalf("absent tags=%v err=%v", patched.TagIDs, err)
	}
	empty := []int64{}
	cleared, err := operations.Patch(ctx, "20000000-0000-4000-8000-000000000003", article.Patch{ID: first.ID, Version: patched.Version, TagIDs: &empty})
	if err != nil || len(cleared.TagIDs) != 0 {
		t.Fatalf("empty tags=%v err=%v", cleared.TagIDs, err)
	}
	if err := fixture.db.GORM().Exec(`UPDATE article_operations SET succeeded_at=clock_timestamp()-interval '25 hours', expires_at=clock_timestamp()-interval '1 hour' WHERE operation_id=?`, key).Error; err != nil {
		t.Fatal(err)
	}
	second, err := operations.Create(ctx, key, operationCreate(fixture, "after-expiry"))
	if err != nil || second.ID == first.ID {
		t.Fatalf("expired replay=%+v err=%v", second, err)
	}
	removed, err := ledger.CleanupExpired(ctx, 100)
	if err != nil || removed != 0 {
		t.Fatalf("cleanup removed replacement: %d %v", removed, err)
	}
	replay, err := operations.Create(ctx, key, article.Create{})
	if err != nil || replay.ID != second.ID {
		t.Fatalf("replacement replay=%+v err=%v", replay, err)
	}
}

func TestArticleOperationsCurrentAccess(t *testing.T) {
	fixture, operations, articles, _, policy := operationFixture(t)
	ctx := context.Background()
	key := "30000000-0000-4000-8000-000000000001"
	first, err := operations.Create(ctx, key, operationCreate(fixture, "access"))
	if err != nil {
		t.Fatal(err)
	}
	policy.denied.Store(true)
	_, err = operations.Create(ctx, key, article.Create{})
	var appErr *application.Error
	if !errors.As(err, &appErr) || appErr.Code != "permission_denied" {
		t.Fatalf("permission replay=%v", err)
	}
	policy.denied.Store(false)
	if err := articles.Delete(ctx, article.Delete{ID: first.ID, Version: first.Version}); err != nil {
		t.Fatal(err)
	}
	_, err = operations.Create(ctx, key, article.Create{})
	if !errors.As(err, &appErr) || appErr.Code != "not_found" {
		t.Fatalf("deleted replay=%v", err)
	}
}

func TestArticleOperationsExpiryCleanup(t *testing.T) {
	fixture, operations, _, ledger, _ := operationFixture(t)
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("40000000-0000-4000-8000-%012d", i)
		if _, err := operations.Create(ctx, key, operationCreate(fixture, fmt.Sprintf("cleanup-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	if err := fixture.db.GORM().Exec(`UPDATE article_operations SET succeeded_at=clock_timestamp()-interval '25 hours', expires_at=clock_timestamp()-interval '1 hour'`).Error; err != nil {
		t.Fatal(err)
	}
	count, err := ledger.CleanupExpired(ctx, 2)
	if err != nil || count != 2 {
		t.Fatalf("bounded cleanup=%d %v", count, err)
	}
	count, err = ledger.CleanupExpired(ctx, 2)
	if err != nil || count != 1 {
		t.Fatalf("remaining cleanup=%d %v", count, err)
	}
}
