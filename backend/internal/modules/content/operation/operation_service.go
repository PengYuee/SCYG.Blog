package operation

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Service owns operation arbitration and bounded expired-success cleanup.
type Service struct{ db *gorm.DB }

// New constructs the success-ledger feature.
func New(db *gorm.DB) (*Service, error) {
	if db == nil {
		return nil, errors.New("operation database is nil")
	}
	return &Service{db: db}, nil
}

// ReserveInTx locks the global key and returns the current successful binding.
func (service *Service) ReserveInTx(ctx context.Context, tx *gorm.DB, key string) (int64, bool, error) {
	if tx == nil {
		return 0, false, errors.New("operation transaction is nil")
	}
	return (repository{db: tx}).reserve(ctx, key)
}

// CompleteInTx associates the reservation with its successfully written article.
func (service *Service) CompleteInTx(ctx context.Context, tx *gorm.DB, key string, articleID int64) error {
	return (repository{db: tx}).complete(ctx, key, articleID)
}

// CleanupExpired removes at most limit expired successes, without external I/O.
func (service *Service) CleanupExpired(ctx context.Context, limit int) (int64, error) {
	if limit < 1 || limit > 1000 {
		return 0, errors.New("operation cleanup limit must be between 1 and 1000")
	}
	return (repository{db: service.db}).cleanup(ctx, limit)
}
