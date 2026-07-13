//go:build integration

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func seedTransitionImages(t *testing.T, fixture repositoryFixture) (domain.ArticleImageID, domain.StorageKey, domain.ArticleImageID, domain.StorageKey) {
	t.Helper()
	firstID, _ := domain.NewArticleImageID("11111111111111111111111111111111")
	firstKey, _ := domain.NewStorageKey(firstID.String() + ".jpg")
	secondID, _ := domain.NewArticleImageID("22222222222222222222222222222222")
	secondKey, _ := domain.NewStorageKey(secondID.String() + ".jpg")
	owner, _ := domain.NewImageOwnerID("abcdef0123456789abcdef0123456789")
	for _, metadata := range []domain.ArticleImageMetadata{
		{ID: firstID, OwnerID: owner, StorageKey: firstKey, MediaType: domain.MediaTypeJPEG, ByteSize: 4, Width: 1, Height: 1, SHA256: strings.Repeat("a", 64)},
		{ID: secondID, OwnerID: owner, StorageKey: secondKey, MediaType: domain.MediaTypeJPEG, ByteSize: 4, Width: 1, Height: 1, SHA256: strings.Repeat("b", 64)},
	} {
		image, err := domain.NewArticleImage(metadata, fixture.clock.now, fixture.clock.now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if err = (&articleImageRepository{db: fixture.db.GORM()}).Save(context.Background(), image); err != nil {
			t.Fatal(err)
		}
	}
	return firstID, firstKey, secondID, secondKey
}

func transactionBackendPID(transaction *gorm.DB) (int, error) {
	var pid int
	err := transaction.Raw("SELECT pg_backend_pid()").Scan(&pid).Error
	return pid, err
}

func Test_Repository_LockReferenceTransition_waiters_share_global_minimum_image_lock(t *testing.T) {
	// Given
	fixture := openRepositoryFixture(t)
	firstID, firstKey, secondID, secondKey := seedTransitionImages(t, fixture)
	blocker := fixture.db.GORM().Begin()
	if blocker.Error != nil {
		t.Fatal(blocker.Error)
	}
	if err := blocker.Exec("SELECT id FROM article_images WHERE id = ? FOR UPDATE", firstID.String()).Error; err != nil {
		_ = blocker.Rollback()
		t.Fatal(err)
	}
	type transition struct {
		oldID domain.ArticleImageID
		key   domain.StorageKey
	}
	started := make(chan int, 2)
	results := make(chan error, 2)
	for _, request := range []transition{{oldID: secondID, key: firstKey}, {oldID: firstID, key: secondKey}} {
		go func() {
			transaction := fixture.db.GORM().Begin()
			if transaction.Error != nil {
				results <- transaction.Error
				return
			}
			pid, pidErr := transactionBackendPID(transaction)
			if pidErr != nil {
				_ = transaction.Rollback()
				results <- pidErr
				return
			}
			started <- pid
			_, err := (&articleImageRepository{db: transaction}).LockReferenceTransition(context.Background(), []domain.ArticleImageID{request.oldID}, []domain.StorageKey{request.key})
			if err != nil {
				_ = transaction.Rollback()
				results <- err
				return
			}
			results <- transaction.Commit().Error
		}()
	}
	pids := []int{<-started, <-started}

	// When
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	observedWaiters := 0
	for observedWaiters < 2 {
		select {
		case <-deadline.C:
			_ = blocker.Rollback()
			t.Fatalf("等待同一最小图片锁的会话不足：%d", observedWaiters)
		case <-ticker.C:
			if err := fixture.db.GORM().Raw("SELECT count(*) FROM pg_stat_activity WHERE pid IN ? AND wait_event_type = 'Lock'", pids).Scan(&observedWaiters).Error; err != nil {
				_ = blocker.Rollback()
				t.Fatal(err)
			}
		}
	}
	if err := blocker.Commit().Error; err != nil {
		t.Fatal(err)
	}

	// Then
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("释放最小图片锁后 transition 未完成")
		}
	}
}

func Test_Repository_legacy_two_phase_cross_lock_deterministically_detects_deadlock(t *testing.T) {
	// Given
	fixture := openRepositoryFixture(t)
	firstID, _, secondID, _ := seedTransitionImages(t, fixture)
	firstTransaction := fixture.db.GORM().Begin()
	secondTransaction := fixture.db.GORM().Begin()
	if firstTransaction.Error != nil || secondTransaction.Error != nil {
		t.Fatal(errors.Join(firstTransaction.Error, secondTransaction.Error))
	}
	if err := firstTransaction.Exec("SELECT id FROM article_images WHERE id = ? FOR UPDATE", secondID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := secondTransaction.Exec("SELECT id FROM article_images WHERE id = ? FOR UPDATE", firstID.String()).Error; err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	requestOther := func(transaction *gorm.DB, id domain.ArticleImageID) {
		<-start
		err := transaction.Exec("SELECT id FROM article_images WHERE id = ? FOR UPDATE", id.String()).Error
		if err != nil {
			_ = transaction.Rollback()
			results <- err
			return
		}
		results <- transaction.Rollback().Error
	}
	go requestOther(firstTransaction, firstID)
	go requestOther(secondTransaction, secondID)

	// When
	close(start)
	outcomes := []error{<-results, <-results}

	// Then
	deadlocks := 0
	successes := 0
	for _, outcome := range outcomes {
		var postgresError *pgconn.PgError
		switch {
		case errors.As(outcome, &postgresError) && postgresError.Code == "40P01":
			deadlocks++
		case outcome == nil:
			successes++
		default:
			t.Fatalf("unexpected cross-lock outcome: %v", outcome)
		}
	}
	if deadlocks != 1 || successes != 1 {
		t.Fatalf("deadlocks=%d successes=%d", deadlocks, successes)
	}
}
