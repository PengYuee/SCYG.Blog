//go:build integration

package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
)

func TestArticleOperationsRechecksWritePermissionAfterPreparation(t *testing.T) {
	fixture, operations, _, ledger, policy := operationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	key := "80000000-0000-4000-8000-000000000001"
	blocker := fixture.db.GORM().WithContext(ctx).Begin()
	if blocker.Error != nil {
		t.Fatal(blocker.Error)
	}
	defer blocker.Rollback()
	if _, replay, err := ledger.ReserveInTx(ctx, blocker, key); err != nil || replay {
		t.Fatalf("reservation=%t %v", replay, err)
	}
	type outcome struct {
		value article.Result
		err   error
	}
	finished := make(chan outcome, 1)
	go func() {
		value, err := operations.Create(ctx, key, operationCreate(fixture, "permission-after-prepare"))
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
			t.Fatalf("writer bypassed arbitration: %+v %v", result.value, result.err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	// Preparation succeeded while allowed. Revocation before the key becomes
	// available must be observed by the actual new-write authorization.
	policy.denied.Store(true)
	if err := blocker.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-finished:
		var appErr *application.Error
		if !errors.As(result.err, &appErr) || appErr.Code != "permission_denied" || result.value.ID != 0 {
			t.Fatalf("write after revocation=%+v %v", result.value, result.err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	policy.denied.Store(false)
	retried, err := operations.Create(ctx, key, operationCreate(fixture, "permission-after-prepare"))
	if err != nil || retried.ID == 0 {
		t.Fatalf("retry=%+v %v", retried, err)
	}
}
