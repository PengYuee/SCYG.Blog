//go:build integration

package user

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	platformdatabase "github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	qadatabase "github.com/PengYuee/SCYG.Blog/backend/internal/qa/database"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

func openIntegrationRepository(t *testing.T) *Repository {
	t.Helper()
	configPath := os.Getenv("QA_CONFIG")
	if configPath == "" {
		t.Fatal("QA_CONFIG is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	isolated, err := qadatabase.New(ctx, configPath, "identity_user_")
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
		_ = pool.Close()
		t.Fatal(err)
	}
	if err = runner.Up(); err != nil {
		_ = runner.Close()
		_ = pool.Close()
		t.Fatal(err)
	}
	if err = runner.Close(); err != nil {
		_ = pool.Close()
		t.Fatal(err)
	}
	if err = pool.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := platformdatabase.New(ctx, platformdatabase.Options{DSN: isolated.DSN(), Logger: slog.Default(), MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.GORM().Exec(`UPDATE users SET is_active = true WHERE username = ?`, "admin").Error; err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(db.GORM())
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestRepositoryFindActiveByUsername_postgres(t *testing.T) {
	repo := openIntegrationRepository(t)
	value, err := repo.FindActiveByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if value.ID.String() != "00000000000000000000000000000001" || value.Username != "admin" || !value.Active {
		t.Fatalf("user = %+v", value)
	}
	if err := value.VerifyPassword("666666"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindActiveByUsername(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("missing user error = %v", err)
	}
}
