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
	"gorm.io/gorm"

	db "github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

type tag struct {
	ID        int64      `gorm:"column:Id;primaryKey"`
	Name      string     `gorm:"column:Name"`
	Version   int64      `gorm:"column:Version"`
	Deleted   *time.Time `gorm:"column:DeletionTime"`
	IsDeleted bool       `gorm:"column:IsDeleted"`
}

func (tag) TableName() string { return "Tag" }
func open(t *testing.T) *db.Database {
	t.Helper()
	dsn := os.Getenv("SCYG_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Fatal("SCYG_TEST_DATABASE_DSN is required")
	}
	value, err := db.New(context.Background(), db.Options{DSN: dsn, Logger: slog.Default(), MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	resetDatabase(t, value)
	t.Cleanup(func() {
		resetDatabase(t, value)
		if err := value.Close(); err != nil {
			t.Error(err)
		}
	})
	return value
}

func resetDatabase(t *testing.T, database *db.Database) {
	t.Helper()
	const resetSQL = `DROP TABLE IF EXISTS "RecoveryProof", "Broken", "TagArticle", "Article", "Tag", "ArticleType", schema_migrations CASCADE`
	if err := database.GORM().Exec(resetSQL).Error; err != nil {
		t.Fatalf("reset isolated test database: %v", err)
	}
}

func mr(t *testing.T, d *db.Database) *migrations.Runner {
	t.Helper()
	pool, e := sql.Open("pgx", os.Getenv("SCYG_TEST_DATABASE_DSN"))
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

func up(t *testing.T, d *db.Database) {
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
	for _, name := range []string{"ArticleType", "Tag", "Article", "TagArticle"} {
		var n int64
		e := d.GORM().Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=?`, name).Scan(&n).Error
		if e != nil || n != 1 {
			t.Fatalf("%s %d %v", name, n, e)
		}
	}
	var n int64
	e := d.GORM().Raw(`SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname LIKE ANY(ARRAY['UX_%','IX_%'])`).Scan(&n).Error
	if e != nil || n < 7 {
		t.Fatalf("indexes %d %v", n, e)
	}
}

func Test_Transaction_commit_rollback_cancel_constraints(t *testing.T) {
	d := open(t)
	up(t, d)
	u, e := db.NewUnitOfWork(d)
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	if e = u.WithinTransaction(ctx, func(c context.Context, tx *gorm.DB) error {
		return tx.WithContext(c).Exec(`INSERT INTO "Tag" ("Name") VALUES (?)`, "commit").Error
	}); e != nil {
		t.Fatal(e)
	}
	sentinel := errors.New("rollback")
	e = u.WithinTransaction(ctx, func(c context.Context, tx *gorm.DB) error {
		tx.WithContext(c).Exec(`INSERT INTO "Tag" ("Name") VALUES (?)`, "rollback")
		return sentinel
	})
	if !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	var n int64
	d.GORM().Table(`"Tag"`).Where(`"Name"=?`, "rollback").Count(&n)
	if n != 0 {
		t.Fatal("rollback persisted")
	}
	c, cancel := context.WithCancel(ctx)
	cancel()
	if e = u.WithinTransaction(c, func(context.Context, *gorm.DB) error { return nil }); !db.IsCanceled(e) {
		t.Fatal(e)
	}
	d.GORM().Exec(`INSERT INTO "Tag" ("Name") VALUES (?)`, "dupe")
	e = d.GORM().Exec(`INSERT INTO "Tag" ("Name") VALUES (?)`, "dupe").Error
	if !db.IsUnique(db.TranslateError(e)) {
		t.Fatal(e)
	}
	e = d.GORM().Exec(`INSERT INTO "Article" ("ArticleTypeId","Title","Slug","Digest","Content") VALUES (999,'t','s','d','c')`).Error
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
	r := d.GORM().Model(&tag{}).Where(`"Id"=? AND "Version"=? AND "IsDeleted"=false`, row.ID, 1).Updates(map[string]any{"Name": "v2", "Version": gorm.Expr(`"Version"+1`)})
	if r.Error != nil || r.RowsAffected != 1 {
		t.Fatal(r.Error)
	}
	r = d.GORM().Model(&tag{}).Where(`"Id"=? AND "Version"=? AND "IsDeleted"=false`, row.ID, 1).Update("Name", "stale")
	if r.Error != nil || r.RowsAffected != 0 {
		t.Fatal("stale update")
	}
	now := time.Now()
	r = d.GORM().Model(&tag{}).Where(`"Id"=? AND "Version"=? AND "IsDeleted"=false`, row.ID, 2).Updates(map[string]any{"IsDeleted": true, "DeletionTime": now, "Version": gorm.Expr(`"Version"+1`)})
	if r.Error != nil || r.RowsAffected != 1 {
		t.Fatal(r.Error)
	}
}

func Test_InvalidMigration_dirty_force_recovery(t *testing.T) {
	d := open(t)
	up(t, d)
	v1u, _ := os.ReadFile("../../../migrations/000001_initial.up.sql")
	v1d, _ := os.ReadFile("../../../migrations/000001_initial.down.sql")
	bad := fstest.MapFS{"000001_initial.up.sql": {Data: v1u}, "000001_initial.down.sql": {Data: v1d}, "000002_bad.up.sql": {Data: []byte(`CREATE TABLE "Broken"("Id" bigint); INVALID SQL;`)}, "000002_bad.down.sql": {Data: []byte(`DROP TABLE "Broken";`)}}
	badPool, e := sql.Open("pgx", os.Getenv("SCYG_TEST_DATABASE_DSN"))
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
	if e != nil || version != 2 || !dirty {
		t.Fatalf("%d %v %v", version, dirty, e)
	}
	var n int64
	d.GORM().Raw(`SELECT count(*) FROM information_schema.tables WHERE table_name='Broken'`).Scan(&n)
	if n != 0 {
		t.Fatal("schema not rolled back")
	}
	if e = r.Force(1); e != nil {
		t.Fatal(e)
	}
	good := fstest.MapFS{"000001_initial.up.sql": {Data: v1u}, "000001_initial.down.sql": {Data: v1d}, "000002_good.up.sql": {Data: []byte(`CREATE TABLE "RecoveryProof" ("Id" bigint PRIMARY KEY);`)}, "000002_good.down.sql": {Data: []byte(`DROP TABLE "RecoveryProof";`)}}
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
	goodPool, e := sql.Open("pgx", os.Getenv("SCYG_TEST_DATABASE_DSN"))
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
