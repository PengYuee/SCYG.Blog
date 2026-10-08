package operation

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type repository struct{ db *gorm.DB }

func (repo repository) success(ctx context.Context, key string) (int64, bool, error) {
	var current record
	err := repo.db.WithContext(ctx).Where("operation_id = ? AND expires_at > clock_timestamp()", key).Take(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if current.ArticleID == nil {
		return 0, false, nil
	}
	return *current.ArticleID, true, nil
}

func (repo repository) reserve(ctx context.Context, key string) (int64, bool, error) {
	// Two-int locks occupy a namespace separate from image bigint locks.
	// Canonical UUID text prevents alternate casing bypassing arbitration.
	if err := repo.db.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(1935898983, hashtext(?::uuid::text))", key).Error; err != nil {
		return 0, false, err
	}
	id, replay, err := repo.success(ctx, key)
	if err != nil || replay {
		return id, replay, err
	}
	err = repo.db.WithContext(ctx).Exec(`INSERT INTO article_operations (operation_id) VALUES (?)
 ON CONFLICT (operation_id) DO UPDATE SET article_id = NULL, succeeded_at = NULL, expires_at = NULL`, key).Error
	return 0, false, err
}

func (repo repository) complete(ctx context.Context, key string, articleID int64) error {
	result := repo.db.WithContext(ctx).Exec(`WITH completion AS (SELECT clock_timestamp() AS at)
	UPDATE article_operations SET article_id = ?, succeeded_at = completion.at, expires_at = completion.at + interval '24 hours'
	FROM completion WHERE operation_id = ? AND succeeded_at IS NULL`, articleID, key)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("operation reservation lost")
	}
	return nil
}

func (repo repository) cleanup(ctx context.Context, limit int) (int64, error) {
	// DELETE locks each selected row and rechecks its expiry after lock waits. A
	// replacement completed meanwhile cannot be removed by a stale candidate.
	result := repo.db.WithContext(ctx).Exec(`DELETE FROM article_operations o USING
 (SELECT operation_id FROM article_operations WHERE expires_at <= clock_timestamp() ORDER BY expires_at, operation_id LIMIT ? FOR UPDATE SKIP LOCKED) candidates
 WHERE o.operation_id = candidates.operation_id AND o.expires_at <= clock_timestamp()`, limit)
	return result.RowsAffected, result.Error
}
