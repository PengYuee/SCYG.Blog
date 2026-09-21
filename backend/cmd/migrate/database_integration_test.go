//go:build integration

package main

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

// Test_run_up_缺库时创建迁移且重复执行成功验证完整迁移命令叙事。
func Test_run_up_缺库时创建迁移且重复执行成功(t *testing.T) {
	// Given
	configPath := os.Getenv("MIGRATION_QUOTED_CONFIG")
	name := os.Getenv("MIGRATION_QUOTED_DATABASE")
	if configPath == "" || name == "" {
		t.Fatal("MIGRATION_QUOTED_CONFIG 和 MIGRATION_QUOTED_DATABASE 必须由 qa-database 提供")
	}
	targetDSN, command, err := loadMigrationConfig([]string{"-config", configPath, "version"})
	if err != nil {
		t.Fatal(err)
	}
	if len(command) != 1 || command[0] != "version" {
		t.Fatal("quoted migration 配置命令解析异常")
	}
	decodedName, err := parseDatabaseName(targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	if decodedName != name {
		t.Fatal("MIGRATION_QUOTED_DATABASE 与临时配置的 database.dsn 不一致")
	}

	// When
	firstErr := run([]string{"-config", configPath, "up"})
	secondErr := run([]string{"-config", configPath, "up"})

	// Then
	if firstErr != nil || secondErr != nil {
		t.Fatalf("首次=%v 再次=%v", firstErr, secondErr)
	}
	pool, err := sql.Open("pgx", targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var version int
	var dirty bool
	if err = pool.QueryRowContext(context.Background(), `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil {
		t.Fatal(err)
	}
	if version != int(migrations.CurrentVersion) || dirty {
		t.Fatalf("version=%d dirty=%t", version, dirty)
	}
	for iteration := 0; iteration < 2; iteration++ {
		if err = run([]string{"-config", configPath, "down"}); err != nil {
			t.Fatalf("第 %d 次回滚迁移失败：%v", iteration+1, err)
		}
		if err = run([]string{"-config", configPath, "up"}); err != nil {
			t.Fatalf("第 %d 次再次迁移失败：%v", iteration+1, err)
		}
	}
	var tableCount, indexCount, pixelCheckCount int
	if err = pool.QueryRowContext(context.Background(), `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('article_images','article_image_references')`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRowContext(context.Background(), `SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname IN ('article_images_status_expires_at_idx','article_images_status_orphaned_at_idx','article_image_references_image_id_idx')`).Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRowContext(context.Background(), `SELECT count(*) FROM pg_constraint WHERE conname='article_images_pixels_check' AND pg_get_constraintdef(oid) LIKE '%width%height%25000000%'`).Scan(&pixelCheckCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 2 || indexCount != 3 || pixelCheckCount != 1 {
		t.Fatalf("图片迁移 catalog 不准确：tables=%d indexes=%d pixel_checks=%d", tableCount, indexCount, pixelCheckCount)
	}
}
