package image

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const cleanupClaimTTL = time.Hour

type cleanupCandidate struct {
	value *ArticleImage
	token string
}

// ClaimExpiredPending claims pending images eligible for cleanup.
func (repo *Repository) ClaimExpiredPending(ctx context.Context, cutoff, now time.Time, limit int) ([]cleanupCandidate, error) {
	return repo.claimExpired(ctx, "pending", cutoff, now, limit)
}

// ClaimExpiredOrphaned claims orphaned images eligible for cleanup.
func (repo *Repository) ClaimExpiredOrphaned(ctx context.Context, cutoff, now time.Time, limit int) ([]cleanupCandidate, error) {
	return repo.claimExpired(ctx, "orphaned", cutoff, now, limit)
}

func (repo *Repository) claimExpired(ctx context.Context, status string, cutoff, now time.Time, limit int) ([]cleanupCandidate, error) {
	if limit < 1 {
		return []cleanupCandidate{}, nil
	}
	token, err := newCleanupClaimToken()
	if err != nil {
		return nil, err
	}
	claimExpiresAt := now.UTC().Add(cleanupClaimTTL)
	var rows []imageModel
	err = repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var selected []imageModel
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status = ? AND expires_at <= ? AND (cleanup_claim_token IS NULL OR cleanup_claim_expires_at <= ?)", status, cutoff.UTC(), now.UTC()).Order("expires_at ASC, id ASC").Limit(limit).Find(&selected).Error; err != nil {
			return translateDatabase(err)
		}
		eligible := selected[:0]
		for _, row := range selected {
			if status == string(ArticleImageStatusOrphaned) {
				id, err := NewArticleImageID(row.ID)
				if err != nil {
					return internal(fmt.Errorf("map cleanup candidate image id: %w", err))
				}
				count, err := (&Repository{db: tx}).CountReferencesForLockedImage(ctx, id)
				if err != nil {
					return err
				}
				if count > 0 {
					continue
				}
			}
			eligible = append(eligible, row)
		}
		if len(eligible) == 0 {
			return nil
		}
		ids := make([]string, len(eligible))
		for index := range eligible {
			ids[index] = eligible[index].ID
		}
		result := tx.WithContext(ctx).Model(&imageModel{}).Where("id IN ?", ids).Updates(map[string]any{
			"cleanup_claim_token":      token,
			"cleanup_claim_expires_at": claimExpiresAt,
		})
		if result.Error != nil {
			return translateDatabase(result.Error)
		}
		if result.RowsAffected != int64(len(eligible)) {
			return internal(errors.New("图片清理候选领取数量不一致"))
		}
		rows = eligible
		return nil
	})
	if err != nil {
		return nil, err
	}
	values, err := mapRows(rows)
	if err != nil {
		return nil, err
	}
	result := make([]cleanupCandidate, len(values))
	for index, value := range values {
		result[index] = cleanupCandidate{value: value, token: token}
	}
	return result, nil
}

func newCleanupClaimToken() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", internal(err)
	}
	return hex.EncodeToString(value), nil
}

var errCleanupClaimNotFound = errors.New("cleanup claim not found")

func (repo *Repository) findCleanupClaimForUpdate(ctx context.Context, id ArticleImageID, token string) (*ArticleImage, error) {
	var row imageModel
	result := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND cleanup_claim_token = ?", id.String(), token).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, errCleanupClaimNotFound
	}
	if result.Error != nil {
		return nil, translateDatabase(result.Error)
	}
	value, err := imageFromModel(row)
	if err != nil {
		return nil, fmt.Errorf("map cleanup claim image: %w", err)
	}
	return value, nil
}

// ReleaseCleanupClaim makes a failed Blob deletion eligible for a later pass.
func (repo *Repository) ReleaseCleanupClaim(ctx context.Context, id ArticleImageID, token string) error {
	return translateDatabase(repo.db.WithContext(ctx).Model(&imageModel{}).Where("id = ? AND cleanup_claim_token = ?", id.String(), token).Updates(map[string]any{
		"cleanup_claim_token":      nil,
		"cleanup_claim_expires_at": nil,
	}).Error)
}

// DeleteClaimedMetadata removes metadata after the external Blob deletion succeeds.
func (repo *Repository) DeleteClaimedMetadata(ctx context.Context, id ArticleImageID, token string, expected ArticleImageStatus) error {
	result := repo.db.WithContext(ctx).Where("id = ? AND status = ? AND cleanup_claim_token = ?", id.String(), string(expected), token).Delete(&imageModel{})
	if result.Error != nil {
		return translateDatabase(result.Error)
	}
	if result.RowsAffected == 0 {
		return internal(errors.New("图片清理领取已失效"))
	}
	return nil
}
