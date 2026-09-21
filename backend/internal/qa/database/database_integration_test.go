//go:build integration

package database

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	qaconfig "github.com/PengYuee/SCYG.Blog/backend/internal/qa/config"
)

func Test_Isolated_Close_removes_every_created_database(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	configPath := os.Getenv("QA_CONFIG")
	if configPath == "" {
		t.Fatal("QA_CONFIG is required")
	}
	first, err := New(ctx, configPath, "database_probe_")
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(ctx, configPath, "content_pg_")
	if err != nil {
		_ = first.Close(ctx)
		t.Fatal(err)
	}
	names := []string{first.name, second.name}
	if err = first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err = second.Close(ctx); err != nil {
		t.Fatal(err)
	}
	config, err := qaconfig.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("pgx", config.AdminDSN().Value())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var remaining int
	if err = admin.QueryRowContext(ctx, `SELECT count(*) FROM pg_database WHERE datname=ANY($1)`, names).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("QA 数据库残留：%d", remaining)
	}
}
