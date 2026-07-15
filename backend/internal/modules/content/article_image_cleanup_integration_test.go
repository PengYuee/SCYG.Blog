//go:build integration

package content_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Test_CleanupArticleImages_removes_expired_pending_missing_file_and_temp_with_real_adapters 验证真实 FS 与 PostgreSQL 收敛。
func Test_CleanupArticleImages_removes_expired_pending_missing_file_and_temp_with_real_adapters(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	first := uploadReferenceImage(t, fixture.module)
	missing := uploadReferenceImage(t, fixture.module)
	now := time.Now().UTC()
	createdAt, past := now.Add(-2*time.Hour), now.Add(-time.Hour)
	if err := fixture.database.GORM().Table("article_images").Where("id IN ?", []string{first.ID, missing.ID}).Updates(map[string]any{"created_at": createdAt, "expires_at": past}).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(fixture.root, missing.StorageKey)); err != nil {
		t.Fatal(err)
	}
	token, _, err := fixture.storage.WriteTemp(context.Background(), "33333333333333333333333333333333", bytes.NewReader([]byte("temp")))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(filepath.Join(fixture.root, token.Name()), past.Add(-24*time.Hour), past.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var beforeRows int64
	if err = fixture.database.GORM().Table("article_images").Where("id IN ?", []string{first.ID, missing.ID}).Count(&beforeRows).Error; err != nil {
		t.Fatal(err)
	}
	_, firstBeforeErr := os.Stat(filepath.Join(fixture.root, first.StorageKey))
	_, tempBeforeErr := os.Stat(filepath.Join(fixture.root, token.Name()))
	t.Logf("QA 删除前：rows=%d final_exists=%t missing_exists=false temp_exists=%t", beforeRows, firstBeforeErr == nil, tempBeforeErr == nil)

	// When
	err = fixture.module.CleanupArticleImages(context.Background())
	// Then
	if err != nil {
		t.Fatal(err)
	}
	var rows int64
	if err = fixture.database.GORM().Table("article_images").Where("id IN ?", []string{first.ID, missing.ID}).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("过期元数据仍存在：%d", rows)
	}
	for _, path := range []string{filepath.Join(fixture.root, first.StorageKey), filepath.Join(fixture.root, token.Name())} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("清理后文件仍存在：%s err=%v", path, statErr)
		}
	}
	t.Logf("QA 删除后：rows=%d final_exists=false missing_exists=false temp_exists=false", rows)
}

// Test_CleanupArticleImages_keeps_metadata_on_file_failure_then_recovers 验证文件失败不删除数据库行。
func Test_CleanupArticleImages_keeps_metadata_on_file_failure_then_recovers(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	image := uploadReferenceImage(t, fixture.module)
	path := filepath.Join(fixture.root, image.StorageKey)
	now := time.Now().UTC()
	if err := fixture.database.GORM().Table("article_images").Where("id = ?", image.ID).Updates(map[string]any{"created_at": now.Add(-2 * time.Hour), "expires_at": now.Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "阻止删除"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	firstErr := fixture.module.CleanupArticleImages(context.Background())
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	secondErr := fixture.module.CleanupArticleImages(context.Background())

	// Then
	var rows int64
	if err := fixture.database.GORM().Table("article_images").Where("id = ?", image.ID).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if firstErr == nil || secondErr != nil || rows != 0 {
		t.Fatalf("firstErr=%v secondErr=%v rows=%d", firstErr, secondErr, rows)
	}
}

// Test_CleanupArticleImages_removes_expired_unreferenced_orphan_with_real_adapters 验证 orphaned grace 到期回收。
func Test_CleanupArticleImages_removes_expired_unreferenced_orphan_with_real_adapters(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	image := uploadReferenceImage(t, fixture.module)
	now := time.Now().UTC()
	state := map[string]any{"status": "orphaned", "created_at": now.Add(-4 * time.Hour), "committed_at": now.Add(-3 * time.Hour), "orphaned_at": now.Add(-2 * time.Hour), "expires_at": now.Add(-time.Hour)}
	if err := fixture.database.GORM().Table("article_images").Where("id = ?", image.ID).Updates(state).Error; err != nil {
		t.Fatal(err)
	}

	// When
	err := fixture.module.CleanupArticleImages(context.Background())

	// Then
	var rows int64
	if queryErr := fixture.database.GORM().Table("article_images").Where("id = ?", image.ID).Count(&rows).Error; queryErr != nil {
		t.Fatal(queryErr)
	}
	if err != nil || rows != 0 {
		t.Fatalf("err=%v rows=%d", err, rows)
	}
}

// Test_CleanupArticleImages_retries_DB_delete_failure_after_file_removal 验证文件已删、DB 失败后的幂等收敛。
func Test_CleanupArticleImages_retries_DB_delete_failure_after_file_removal(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	image := uploadReferenceImage(t, fixture.module)
	now := time.Now().UTC()
	if err := fixture.database.GORM().Table("article_images").Where("id = ?", image.ID).Updates(map[string]any{"created_at": now.Add(-2 * time.Hour), "expires_at": now.Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	trigger := `CREATE FUNCTION reject_task8_image_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION '删除元数据失败'; END $$; CREATE TRIGGER task8_reject_image_delete BEFORE DELETE ON article_images FOR EACH ROW EXECUTE FUNCTION reject_task8_image_delete()`
	if err := fixture.database.GORM().Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}

	// When
	firstErr := fixture.module.CleanupArticleImages(context.Background())
	var rowsAfterFirst int64
	if err := fixture.database.GORM().Table("article_images").Where("id = ?", image.ID).Count(&rowsAfterFirst).Error; err != nil {
		t.Fatal(err)
	}
	_, fileAfterFirstErr := os.Stat(filepath.Join(fixture.root, image.StorageKey))
	if firstErr == nil || rowsAfterFirst != 1 || !os.IsNotExist(fileAfterFirstErr) {
		t.Fatalf("首轮状态错误：err=%v rows=%d fileErr=%v", firstErr, rowsAfterFirst, fileAfterFirstErr)
	}
	if err := fixture.database.GORM().Exec(`DROP TRIGGER task8_reject_image_delete ON article_images; DROP FUNCTION reject_task8_image_delete()`).Error; err != nil {
		t.Fatal(err)
	}
	secondErr := fixture.module.CleanupArticleImages(context.Background())

	// Then
	var rows int64
	if err := fixture.database.GORM().Table("article_images").Where("id = ?", image.ID).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(fixture.root, image.StorageKey)); !os.IsNotExist(statErr) {
		t.Fatalf("文件未删除：%v", statErr)
	}
	if firstErr == nil || secondErr != nil || rows != 0 {
		t.Fatalf("firstErr=%v secondErr=%v rows=%d", firstErr, secondErr, rows)
	}
}

// Test_CleanupArticleImages_and_reference_compete_without_dangling_reference 验证真实清理与重新引用竞争保持一致性。
func Test_CleanupArticleImages_and_reference_compete_without_dangling_reference(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	image := uploadReferenceImage(t, fixture.module)
	now := time.Now().UTC()
	state := map[string]any{"status": "orphaned", "created_at": now.Add(-4 * time.Hour), "committed_at": now.Add(-3 * time.Hour), "orphaned_at": now.Add(-2 * time.Hour), "expires_at": now.Add(-time.Nanosecond)}
	if err := fixture.database.GORM().Table("article_images").Where("id = ?", image.ID).Updates(state).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := fixture.database.GORM().DB()
	if err != nil {
		t.Fatal(err)
	}
	lockConnection, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer lockConnection.Close()
	const referenceBarrierID, cleanupBarrierID = 88007, 88008
	if _, err = lockConnection.ExecContext(context.Background(), `SELECT pg_advisory_lock($1), pg_advisory_lock($2)`, referenceBarrierID, cleanupBarrierID); err != nil {
		t.Fatal(err)
	}
	trigger := `CREATE FUNCTION pause_task8_reference() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(88007); RETURN NEW; END $$;
CREATE TRIGGER task8_pause_reference BEFORE INSERT ON "Article" FOR EACH ROW EXECUTE FUNCTION pause_task8_reference();
CREATE FUNCTION pause_task8_cleanup() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(88008); RETURN OLD; END $$;
CREATE TRIGGER task8_pause_cleanup BEFORE DELETE ON article_images FOR EACH ROW EXECUTE FUNCTION pause_task8_cleanup()`
	if err = fixture.database.GORM().Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = fixture.database.GORM().Exec(`DROP TRIGGER IF EXISTS task8_pause_reference ON "Article"; DROP FUNCTION IF EXISTS pause_task8_reference(); DROP TRIGGER IF EXISTS task8_pause_cleanup ON article_images; DROP FUNCTION IF EXISTS pause_task8_cleanup()`).Error
	}()
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1), pg_advisory_unlock($2)`, referenceBarrierID, cleanupBarrierID)
	}()
	cleanupDone, referenceDone := make(chan error, 1), make(chan error, 1)

	// When
	go func() {
		_, err := createReferenceArticle(t, fixture.module, "cleanup-race", "![竞态]("+image.URL+")")
		referenceDone <- err
	}()
	waitForCleanupLock(t, fixture, "reference-barrier")
	go func() {
		cleanupDone <- fixture.module.CleanupArticleImages(context.Background())
	}()
	waitForCleanupLock(t, fixture, "cleanup-barrier")
	if _, err = lockConnection.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, referenceBarrierID); err != nil {
		t.Fatal(err)
	}
	waitForCleanupLock(t, fixture, "reference")
	if _, err = lockConnection.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, cleanupBarrierID); err != nil {
		t.Fatal(err)
	}
	cleanupErr, referenceErr := <-cleanupDone, <-referenceDone

	// Then
	var metadata, references int64
	if err = fixture.database.GORM().Table("article_images").Where("id = ?", image.ID).Count(&metadata).Error; err != nil {
		t.Fatal(err)
	}
	if err = fixture.database.GORM().Table("article_image_references").Where("image_id = ?", image.ID).Count(&references).Error; err != nil {
		t.Fatal(err)
	}
	_, fileErr := os.Stat(filepath.Join(fixture.root, image.StorageKey))
	if referenceErr == nil {
		var status string
		if err = fixture.database.GORM().Table("article_images").Select("status").Where("id = ?", image.ID).Scan(&status).Error; err != nil {
			t.Fatal(err)
		}
		if cleanupErr != nil || metadata != 1 || references != 1 || status != "committed" || fileErr != nil {
			t.Fatalf("引用获胜状态非法：cleanupErr=%v metadata=%d references=%d status=%s fileErr=%v", cleanupErr, metadata, references, status, fileErr)
		}
		return
	}
	if cleanupErr != nil || metadata != 0 || references != 0 || !os.IsNotExist(fileErr) {
		t.Fatalf("清理获胜状态非法：cleanupErr=%v referenceErr=%v metadata=%d references=%d fileErr=%v", cleanupErr, referenceErr, metadata, references, fileErr)
	}
}

// waitForCleanupLock 通过 pg_locks/pg_stat_activity 观察两个事务确实在数据库锁上相遇。
func waitForCleanupLock(t *testing.T, fixture articleReferenceFixture, phase string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int64
		var err error
		if phase == "reference-barrier" {
			err = fixture.database.GORM().Raw(`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND objid = 88007 AND NOT granted`).Scan(&count).Error
		} else if phase == "cleanup-barrier" {
			err = fixture.database.GORM().Raw(`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND objid = 88008 AND NOT granted`).Scan(&count).Error
		} else {
			err = fixture.database.GORM().Raw(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND wait_event <> 'advisory' AND pid <> pg_backend_pid()`).Scan(&count).Error
		}
		if err != nil {
			t.Fatal(err)
		}
		if count > 0 {
			return
		}
		select {
		case <-ctx.Done():
			var activities []struct {
				PID       int
				WaitEvent string
				Query     string
			}
			_ = fixture.database.GORM().Raw(`SELECT pid, coalesce(wait_event, '') AS wait_event, query FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid()`).Scan(&activities).Error
			t.Fatalf("等待 %s 锁事件超时：%v activities=%+v", phase, ctx.Err(), activities)
		case <-ticker.C:
		}
	}
}
