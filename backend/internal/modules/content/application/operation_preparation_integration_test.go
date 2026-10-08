//go:build integration

package application_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
)

func TestArticleOperationsConcurrentSuccessWinsPreparationErrors(t *testing.T) {
	fixture, operations, _, ledger, _ := operationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	first, err := operations.Create(ctx, "70000000-0000-4000-8000-000000000001", operationCreate(fixture, "prepared-existing"))
	if err != nil {
		t.Fatal(err)
	}
	scenarios := []struct {
		name   string
		invoke func(string) (article.Result, error)
	}{
		{name: "invalid-title", invoke: func(key string) (article.Result, error) {
			input := operationCreate(fixture, "ignored")
			input.Title = ""
			return operations.Create(ctx, key, input)
		}},
		{name: "missing-image", invoke: func(key string) (article.Result, error) {
			input := operationCreate(fixture, "ignored")
			input.Content = "![missing](/media/article-images/missing.jpg)"
			return operations.Create(ctx, key, input)
		}},
		{name: "invalid-version", invoke: func(key string) (article.Result, error) {
			return operations.Patch(ctx, key, article.Patch{ID: first.ID, Version: 0, Title: new("ignored")})
		}},
	}
	for index, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			key := fmt.Sprintf("70000000-0000-4000-8000-%012d", index+2)
			transaction := fixture.db.GORM().WithContext(ctx).Begin()
			if transaction.Error != nil {
				t.Fatal(transaction.Error)
			}
			defer transaction.Rollback()
			if _, replay, err := ledger.ReserveInTx(ctx, transaction, key); err != nil || replay {
				t.Fatalf("reservation replay=%t err=%v", replay, err)
			}
			type outcome struct {
				value article.Result
				err   error
			}
			finished := make(chan outcome, 1)
			go func() { value, err := scenario.invoke(key); finished <- outcome{value: value, err: err} }()
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
					t.Fatalf("preparation error bypassed arbitration: %+v %v", result.value, result.err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(time.Millisecond):
				}
			}
			if err := ledger.CompleteInTx(ctx, transaction, key, first.ID); err != nil {
				t.Fatal(err)
			}
			if err := transaction.Commit().Error; err != nil {
				t.Fatal(err)
			}
			select {
			case result := <-finished:
				if result.err != nil || result.value.ID != first.ID {
					t.Fatalf("successful replay=%+v err=%v", result.value, result.err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}

func TestArticleImagesPreparedPatchRejectsInterveningVersion(t *testing.T) {
	fixture := newApplicationFixture(t)
	ctx := context.Background()
	first := createApplicationArticle(t, fixture, "preview-fence", "body")
	prepared, err := fixture.workflow.PreparePatch(ctx, article.Patch{ID: first.ID, Version: first.Version, Content: new("prepared-body")})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := fixture.workflow.Patch(ctx, article.Patch{ID: first.ID, Version: first.Version, Content: new("intervening-body")})
	if err != nil {
		t.Fatal(err)
	}
	transaction := fixture.db.GORM().WithContext(ctx).Begin()
	if transaction.Error != nil {
		t.Fatal(transaction.Error)
	}
	defer transaction.Rollback()
	if _, err := fixture.workflow.PatchInTx(ctx, transaction, prepared); err == nil {
		t.Fatal("stale prepared preview was persisted")
	}
	if err := transaction.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	query, err := article.NewQuery(fixture.db.GORM())
	if err != nil {
		t.Fatal(err)
	}
	current, err := query.GetManage(ctx, article.Get{ID: first.ID})
	if err != nil || current.Content != "intervening-body" || current.Version != changed.Version {
		t.Fatalf("fenced current=%+v err=%v", current, err)
	}
}
