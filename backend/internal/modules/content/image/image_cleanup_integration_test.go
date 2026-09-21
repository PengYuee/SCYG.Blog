//go:build integration

package image

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	qadatabase "github.com/PengYuee/SCYG.Blog/backend/internal/qa/database"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

type cleanupIntegrationClock struct{ now time.Time }

func (clock cleanupIntegrationClock) Now() time.Time { return clock.now }

type cleanupIntegrationBlob struct {
	deleted      []string
	deleteErr    error
	beforeDelete func(string) error
}

func (blob *cleanupIntegrationBlob) Stage(context.Context, string, io.Reader) (string, int64, error) {
	return "", 0, nil
}
func (blob *cleanupIntegrationBlob) Commit(string, string) error           { return nil }
func (blob *cleanupIntegrationBlob) Discard(context.Context, string) error { return nil }
func (blob *cleanupIntegrationBlob) Load(string) ([]byte, error)           { return nil, nil }
func (blob *cleanupIntegrationBlob) Delete(key string) error {
	if blob.deleteErr != nil {
		return blob.deleteErr
	}
	if blob.beforeDelete != nil {
		if err := blob.beforeDelete(key); err != nil {
			return err
		}
	}
	blob.deleted = append(blob.deleted, key)
	return nil
}

func (blob *cleanupIntegrationBlob) ListExpiredTemps(context.Context, time.Time, int) ([]string, error) {
	return nil, nil
}
func (blob *cleanupIntegrationBlob) DeleteTemp(context.Context, string) error { return nil }

type cleanupIntegrationFixture struct {
	db  *database.Database
	now time.Time
}

func openCleanupIntegrationFixture(t *testing.T) cleanupIntegrationFixture {
	t.Helper()
	configPath := os.Getenv("QA_CONFIG")
	if configPath == "" {
		t.Fatal("QA_CONFIG is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	isolated, err := qadatabase.New(ctx, configPath, "image_cleanup_")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := isolated.Close(cleanupCtx); err != nil {
			t.Error(err)
		}
	})
	pool, err := sql.Open("pgx", isolated.DSN())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := migrations.New(pool, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Close(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	db, err := database.New(ctx, database.Options{DSN: isolated.DSN(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.GORM().Exec(`TRUNCATE article_image_references, article_images RESTART IDENTITY CASCADE`).Error; err != nil {
			t.Error(err)
		}
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.GORM().Exec(`TRUNCATE article_image_references, article_images RESTART IDENTITY CASCADE`).Error; err != nil {
		t.Fatal(err)
	}
	return cleanupIntegrationFixture{db: db, now: now}
}

func insertExpiredCleanupImage(t *testing.T, fixture cleanupIntegrationFixture, id, status string) string {
	t.Helper()
	key := id + ".jpg"
	created := fixture.now.Add(-3 * time.Hour)
	var committed, orphaned any
	switch status {
	case string(ArticleImageStatusPending):
	case string(ArticleImageStatusOrphaned):
		committed = fixture.now.Add(-2 * time.Hour)
		orphaned = fixture.now.Add(-90 * time.Minute)
	default:
		t.Fatalf("unsupported cleanup fixture status %q", status)
	}
	err := fixture.db.GORM().Exec(`INSERT INTO article_images (id, owner_id, storage_key, media_type, byte_size, width, height, sha256, status, created_at, committed_at, orphaned_at, expires_at) VALUES (?, ?, ?, 'image/jpeg', 1, 1, 1, ?, ?, ?, ?, ?, ?)`, id, "abcdef0123456789abcdef0123456789", key, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", status, created, committed, orphaned, fixture.now.Add(-time.Minute)).Error
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestCleanup_Postgres_claimsBeforeBlobDeleteAndRemovesMetadataAfterSuccess(t *testing.T) {
	fixture := openCleanupIntegrationFixture(t)
	pendingID := "0123456789abcdef0123456789abcdef"
	orphanedID := "fedcba9876543210fedcba98765432aa"
	pendingKey := insertExpiredCleanupImage(t, fixture, pendingID, string(ArticleImageStatusPending))
	orphanedKey := insertExpiredCleanupImage(t, fixture, orphanedID, string(ArticleImageStatusOrphaned))
	blob := &cleanupIntegrationBlob{
		beforeDelete: func(key string) error {
			var token sql.NullString
			if err := fixture.db.GORM().Table("article_images").Select("cleanup_claim_token").Where("storage_key = ?", key).Take(&token).Error; err != nil {
				return err
			}
			if !token.Valid {
				return errors.New("cleanup claim missing before blob delete")
			}
			return nil
		},
	}
	cleanup, err := NewDatabaseCleanup(fixture.db.GORM(), blob, cleanupIntegrationClock{now: fixture.now}, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanup.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	deleted := map[string]bool{}
	for _, key := range blob.deleted {
		deleted[key] = true
	}
	if len(blob.deleted) != 2 || !deleted[pendingKey] || !deleted[orphanedKey] {
		t.Fatalf("deleted blobs = %#v, want %q and %q", blob.deleted, pendingKey, orphanedKey)
	}
	var count int64
	if err := fixture.db.GORM().Table("article_images").Where("id IN ?", []string{pendingID, orphanedID}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("image metadata count = %d, want 0", count)
	}
}

func TestCleanup_Postgres_releasesClaimWhenBlobDeleteFails(t *testing.T) {
	fixture := openCleanupIntegrationFixture(t)
	id := "fedcba9876543210fedcba9876543210"
	insertExpiredCleanupImage(t, fixture, id, string(ArticleImageStatusOrphaned))
	blob := &cleanupIntegrationBlob{deleteErr: errors.New("blob unavailable")}
	cleanup, err := NewDatabaseCleanup(fixture.db.GORM(), blob, cleanupIntegrationClock{now: fixture.now}, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanup.Run(context.Background()); err == nil {
		t.Fatal("blob failure was swallowed")
	}
	var token sql.NullString
	if err := fixture.db.GORM().Table("article_images").Select("cleanup_claim_token").Where("id = ?", id).Take(&token).Error; err != nil {
		t.Fatal(err)
	}
	if token.Valid {
		t.Fatalf("cleanup claim = %q, want released claim", token.String)
	}
	blob.deleteErr = nil
	if err := cleanup.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := fixture.db.GORM().Table("article_images").Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("image metadata count after retry = %d, want 0", count)
	}
}
