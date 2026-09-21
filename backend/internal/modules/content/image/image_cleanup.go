package image

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

const cleanupBatchSize = 100

// Cleanup removes expired pending images, orphan metadata, and temporary blobs.
type Cleanup struct {
	db         *gorm.DB
	repository *Repository
	blob       Blob
	clock      content.Clock
	policy     Policy
}

// NewCleanup constructs a cleanup runner with explicit collaborators.
func NewCleanup(db *gorm.DB, repository *Repository, blob Blob, clock content.Clock, policy Policy) *Cleanup {
	return &Cleanup{db: db, repository: repository, blob: blob, clock: clock, policy: policy.valid()}
}

// NewDatabaseCleanup constructs the cleanup runner over one GORM handle.
func NewDatabaseCleanup(db *gorm.DB, blob Blob, clock content.Clock, policy Policy) (*Cleanup, error) {
	if db == nil {
		return nil, errors.New("image cleanup database is nil")
	}
	if blob == nil {
		return nil, errors.New("image cleanup blob is nil")
	}
	if clock == nil {
		return nil, errors.New("image cleanup clock is nil")
	}
	repository, err := NewRepository(db)
	if err != nil {
		return nil, err
	}
	return NewCleanup(db, repository, blob, clock, policy), nil
}

// Run executes one bounded image cleanup pass.
func (cleanup *Cleanup) Run(ctx context.Context) error {
	now := cleanup.clock.Now().UTC()
	var failures []error
	for _, status := range []ArticleImageStatus{ArticleImageStatusPending, ArticleImageStatusOrphaned} {
		candidates, err := cleanup.candidates(ctx, status, now)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, candidate := range candidates {
			if err := cleanup.candidate(ctx, candidate, status, now); err != nil {
				failures = append(failures, fmt.Errorf("清理图片 %s：%w", candidate.value.Metadata().ID.String(), err))
			}
		}
	}
	temps, err := cleanup.blob.ListExpiredTemps(ctx, now.Add(-cleanup.policy.PendingTTL()), cleanupBatchSize)
	if err != nil {
		failures = append(failures, fmt.Errorf("枚举过期临时图片：%w", err))
	} else {
		for _, name := range temps {
			if err := cleanup.blob.DeleteTemp(ctx, name); err != nil {
				failures = append(failures, fmt.Errorf("清理临时图片 %s：%w", name, err))
			}
		}
	}
	return errors.Join(failures...)
}

// CleanupArticleImages satisfies the bootstrap cleanup runner contract.
func (cleanup *Cleanup) CleanupArticleImages(ctx context.Context) error { return cleanup.Run(ctx) }

func (cleanup *Cleanup) candidates(ctx context.Context, status ArticleImageStatus, now time.Time) ([]cleanupCandidate, error) {
	switch status {
	case ArticleImageStatusPending:
		return cleanup.repository.ClaimExpiredPending(ctx, now, now, cleanupBatchSize)
	case ArticleImageStatusOrphaned:
		return cleanup.repository.ClaimExpiredOrphaned(ctx, now, now, cleanupBatchSize)
	case ArticleImageStatusCommitted:
		return nil, errors.New("不支持的图片清理状态")
	default:
		return nil, errors.New("不支持的图片清理状态")
	}
}

func (cleanup *Cleanup) candidate(ctx context.Context, candidate cleanupCandidate, expected ArticleImageStatus, now time.Time) error {
	eligible, err := cleanup.validateCandidate(ctx, candidate, expected, now)
	if err != nil || !eligible {
		return err
	}
	if err := cleanup.blob.Delete(candidate.value.Metadata().StorageKey.String()); err != nil {
		return errors.Join(err, cleanup.releaseClaim(ctx, candidate))
	}
	return cleanup.deleteClaimed(ctx, candidate, expected)
}

func (cleanup *Cleanup) validateCandidate(ctx context.Context, candidate cleanupCandidate, expected ArticleImageStatus, now time.Time) (bool, error) {
	eligible := false
	err := cleanup.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &Repository{db: tx}
		value, err := repo.findCleanupClaimForUpdate(ctx, candidate.value.Metadata().ID, candidate.token)
		if errors.Is(err, errCleanupClaimNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if value == nil {
			return errors.New("图片清理认领记录为空")
		}
		if value.Status() != expected || value.ExpiresAt().After(now) {
			return repo.ReleaseCleanupClaim(ctx, candidate.value.Metadata().ID, candidate.token)
		}
		if expected == ArticleImageStatusOrphaned {
			count, err := repo.CountReferencesForLockedImage(ctx, candidate.value.Metadata().ID)
			if err != nil {
				return err
			}
			if count > 0 {
				return repo.ReleaseCleanupClaim(ctx, candidate.value.Metadata().ID, candidate.token)
			}
		}
		eligible = true
		return nil
	})
	return eligible, err
}

func (cleanup *Cleanup) releaseClaim(ctx context.Context, candidate cleanupCandidate) error {
	return cleanup.repository.ReleaseCleanupClaim(ctx, candidate.value.Metadata().ID, candidate.token)
}

func (cleanup *Cleanup) deleteClaimed(ctx context.Context, candidate cleanupCandidate, expected ArticleImageStatus) error {
	return cleanup.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &Repository{db: tx}
		value, err := repo.findCleanupClaimForUpdate(ctx, candidate.value.Metadata().ID, candidate.token)
		if errors.Is(err, errCleanupClaimNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if value == nil {
			return errors.New("图片清理认领记录为空")
		}
		if value.Status() != expected {
			return internal(errors.New("图片清理状态在 Blob 删除期间发生变化"))
		}
		return repo.DeleteClaimedMetadata(ctx, candidate.value.Metadata().ID, candidate.token, expected)
	})
}
