//go:build integration

package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
)

func TestArticleOperationsRechecksExpiryAfterKeyLockWait(t *testing.T) {
	fixture, operations, _, ledger, _ := operationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	key := "60000000-0000-4000-8000-000000000001"
	first, err := operations.Create(ctx, key, operationCreate(fixture, "before-wait"))
	if err != nil {
		t.Fatal(err)
	}
	blocker := fixture.db.GORM().WithContext(ctx).Begin()
	if blocker.Error != nil {
		t.Fatal(blocker.Error)
	}
	defer blocker.Rollback()
	if err := blocker.Exec("SELECT pg_advisory_xact_lock(1935898983, hashtext(?::uuid::text))", key).Error; err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		value article.Result
		err   error
	}
	finished := make(chan outcome, 1)
	go func() {
		value, err := operations.Create(ctx, key, operationCreate(fixture, "after-wait"))
		finished <- outcome{value: value, err: err}
	}()
	for {
		var waiting bool
		if err := fixture.db.GORM().WithContext(ctx).Raw(`SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted AND database=(SELECT oid FROM pg_database WHERE datname=current_database()))`).Scan(&waiting).Error; err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case result := <-finished:
			t.Fatalf("writer did not await key lock: %+v %v", result.value, result.err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	// Change expiry while holding the exact lock awaited by the writer. The next
	// holder must inspect the committed row after it acquires the lock.
	if err := blocker.Exec(`UPDATE article_operations SET succeeded_at=clock_timestamp()-interval '25 hours', expires_at=clock_timestamp()-interval '1 hour' WHERE operation_id=?`, key).Error; err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-finished:
		if result.err != nil || result.value.ID == first.ID {
			t.Fatalf("lock-wait result=%+v err=%v", result.value, result.err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	removed, err := ledger.CleanupExpired(ctx, 100)
	if err != nil || removed != 0 {
		t.Fatalf("cleanup deleted refreshed binding: %d %v", removed, err)
	}
}
