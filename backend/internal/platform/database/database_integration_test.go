//go:build integration

package database_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	db "github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	qadatabase "github.com/PengYuee/SCYG.Blog/backend/internal/qa/database"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

type tag struct {
	ID        int64      `gorm:"column:id;primaryKey"`
	Name      string     `gorm:"column:name"`
	Version   int64      `gorm:"column:version"`
	Deleted   *time.Time `gorm:"column:deleted_at"`
	IsDeleted bool       `gorm:"column:is_deleted"`
}

func (tag) TableName() string { return "tags" }

type isolatedDatabase struct {
	*db.Database
	dsn string
}

func open(t *testing.T) isolatedDatabase {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	configPath := os.Getenv("QA_CONFIG")
	if configPath == "" {
		t.Fatal("QA_CONFIG is required")
	}
	isolated, err := qadatabase.New(ctx, configPath, "database_")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if closeErr := isolated.Close(cleanupCtx); closeErr != nil {
			t.Error(closeErr)
		}
	})
	value, err := db.New(ctx, db.Options{DSN: isolated.DSN(), Logger: slog.Default(), MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := value.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	return isolatedDatabase{Database: value, dsn: isolated.DSN()}
}

func mr(t *testing.T, d isolatedDatabase) *migrations.Runner {
	t.Helper()
	pool, e := sql.Open("pgx", d.dsn)
	if e != nil {
		t.Fatal(e)
	}
	r, e := migrations.New(pool, "")
	if e != nil {
		if closeErr := pool.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if closeErr := r.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	return r
}

func up(t *testing.T, d isolatedDatabase) {
	t.Helper()
	if e := mr(t, d).Up(); e != nil {
		t.Fatal(e)
	}
}

func Test_MigrationRoundTrip_up_down_up(t *testing.T) {
	d := open(t)
	r := mr(t, d)
	if e := r.Up(); e != nil {
		t.Fatal(e)
	}
	if e := r.Down(); e != nil {
		t.Fatal(e)
	}
	if e := r.Up(); e != nil {
		t.Fatal(e)
	}
}

func Test_ExactSchema_catalog(t *testing.T) {
	d := open(t)
	up(t, d)
	for _, name := range []string{"article_types", "tags", "articles", "article_tags", "users"} {
		var n int64
		e := d.GORM().Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=?`, name).Scan(&n).Error
		if e != nil || n != 1 {
			t.Fatalf("%s %d %v", name, n, e)
		}
	}
	var n int64
	e := d.GORM().Raw(`SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname LIKE '%_idx'`).Scan(&n).Error
	if e != nil || n < 8 {
		t.Fatalf("indexes %d %v", n, e)
	}
}

func Test_DefaultUser_passwordHashAndIdempotentMigration(t *testing.T) {
	d := open(t)
	up(t, d)
	var row struct {
		ID           string
		Username     string
		PasswordHash string
		IsActive     bool
	}
	if err := d.GORM().Raw(`SELECT id, username, password_hash, is_active FROM users WHERE username = ?`, "admin").Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.ID != "00000000000000000000000000000001" || row.Username != "admin" || !row.IsActive {
		t.Fatalf("default user = %+v", row)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte("666666")); err != nil {
		t.Fatalf("default password hash does not verify: %v", err)
	}
	if err := mr(t, d).Up(); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := d.GORM().Table("users").Where("username = ?", "admin").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("default user count = %d", count)
	}
}

func Test_Transaction_commit_rollback_cancel_constraints(t *testing.T) {
	d := open(t)
	up(t, d)
	ctx := context.Background()
	if e := d.GORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Exec(`INSERT INTO tags (name) VALUES (?)`, "commit").Error
	}); e != nil {
		t.Fatal(e)
	}
	sentinel := errors.New("rollback")
	e := d.GORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx.WithContext(ctx).Exec(`INSERT INTO tags (name) VALUES (?)`, "rollback")
		return sentinel
	})
	if !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	var n int64
	d.GORM().Table("tags").Where("name = ?", "rollback").Count(&n)
	if n != 0 {
		t.Fatal("rollback persisted")
	}
	c, cancel := context.WithCancel(ctx)
	cancel()
	if e = d.GORM().WithContext(c).Transaction(func(*gorm.DB) error { return nil }); !db.IsCanceled(db.TranslateError(e)) {
		t.Fatal(e)
	}
	d.GORM().Exec(`INSERT INTO tags (name) VALUES (?)`, "dupe")
	e = d.GORM().Exec(`INSERT INTO tags (name) VALUES (?)`, "dupe").Error
	if !db.IsUnique(db.TranslateError(e)) {
		t.Fatal(e)
	}
	e = d.GORM().Exec(`INSERT INTO articles (article_type_id, title, slug, digest, content) VALUES (999,'t','s','d','c')`).Error
	if !db.IsForeignKey(db.TranslateError(e)) {
		t.Fatal(e)
	}
}

func Test_OptimisticUpdate_version_and_soft_delete(t *testing.T) {
	d := open(t)
	up(t, d)
	row := tag{Name: "v", Version: 1}
	if e := d.GORM().Create(&row).Error; e != nil {
		t.Fatal(e)
	}
	r := d.GORM().Model(&tag{}).Where("id = ? AND version = ? AND is_deleted = false", row.ID, 1).Updates(map[string]any{"name": "v2", "version": gorm.Expr("version + 1")})
	if r.Error != nil || r.RowsAffected != 1 {
		t.Fatal(r.Error)
	}
	r = d.GORM().Model(&tag{}).Where("id = ? AND version = ? AND is_deleted = false", row.ID, 1).Update("name", "stale")
	if r.Error != nil || r.RowsAffected != 0 {
		t.Fatal("stale update")
	}
	now := time.Now()
	r = d.GORM().Model(&tag{}).Where("id = ? AND version = ? AND is_deleted = false", row.ID, 2).Updates(map[string]any{"is_deleted": true, "deleted_at": now, "version": gorm.Expr("version + 1")})
	if r.Error != nil || r.RowsAffected != 1 {
		t.Fatal(r.Error)
	}
}

func Test_InvalidMigration_dirty_force_recovery(t *testing.T) {
	d := open(t)
	base := mr(t, d)
	if e := base.Up(); e != nil {
		t.Fatal(e)
	}
	if e := base.Down(); e != nil {
		t.Fatal(e)
	}
	if e := base.Close(); e != nil {
		t.Fatal(e)
	}
	v1u, _ := os.ReadFile("../../../migrations/000001_initial.up.sql")
	v1d, _ := os.ReadFile("../../../migrations/000001_initial.down.sql")
	v2u, _ := os.ReadFile("../../../migrations/000002_article_images.up.sql")
	v2d, _ := os.ReadFile("../../../migrations/000002_article_images.down.sql")
	v3u, _ := os.ReadFile("../../../migrations/000003_article_image_cleanup_claims.up.sql")
	v3d, _ := os.ReadFile("../../../migrations/000003_article_image_cleanup_claims.down.sql")
	bad := fstest.MapFS{
		"000001_initial.up.sql":                        {Data: v1u},
		"000001_initial.down.sql":                      {Data: v1d},
		"000002_article_images.up.sql":                 {Data: v2u},
		"000002_article_images.down.sql":               {Data: v2d},
		"000003_article_image_cleanup_claims.up.sql":   {Data: v3u},
		"000003_article_image_cleanup_claims.down.sql": {Data: v3d},
		"000004_bad.up.sql":                            {Data: []byte(`CREATE TABLE broken(id bigint); INVALID SQL;`)},
		"000004_bad.down.sql":                          {Data: []byte(`DROP TABLE broken;`)},
	}
	badPool, e := sql.Open("pgx", d.dsn)
	if e != nil {
		t.Fatal(e)
	}
	r, e := migrations.NewWithSource(badPool, "", bad)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if closeErr := r.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	if e = r.Up(); e == nil {
		t.Fatal("expected failure")
	}
	version, dirty, e := r.Version()
	if e != nil || version != 4 || !dirty {
		t.Fatalf("%d %v %v", version, dirty, e)
	}
	var n int64
	d.GORM().Raw(`SELECT count(*) FROM information_schema.tables WHERE table_name='Broken'`).Scan(&n)
	if n != 0 {
		t.Fatal("schema not rolled back")
	}
	if e = r.Force(3); e != nil {
		t.Fatal(e)
	}
	good := fstest.MapFS{
		"000001_initial.up.sql":                        {Data: v1u},
		"000001_initial.down.sql":                      {Data: v1d},
		"000002_article_images.up.sql":                 {Data: v2u},
		"000002_article_images.down.sql":               {Data: v2d},
		"000003_article_image_cleanup_claims.up.sql":   {Data: v3u},
		"000003_article_image_cleanup_claims.down.sql": {Data: v3d},
		"000004_good.up.sql":                           {Data: []byte(`CREATE TABLE recovery_proof (id bigint PRIMARY KEY);`)},
		"000004_good.down.sql":                         {Data: []byte(`DROP TABLE recovery_proof;`)},
	}
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
	goodPool, e := sql.Open("pgx", d.dsn)
	if e != nil {
		t.Fatal(e)
	}
	r, e = migrations.NewWithSource(goodPool, "", good)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Up(); e != nil {
		t.Fatal(e)
	}
}
