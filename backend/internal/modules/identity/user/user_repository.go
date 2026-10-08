package user

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
)

// Repository reads users from the users table owned by this feature.
type Repository struct{ db *gorm.DB }

// NewRepository constructs a user repository.
func NewRepository(db *gorm.DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("user repository database is nil")
	}
	return &Repository{db: db}, nil
}

// FindActiveByUsername returns the active account with the supplied username.
func (repo *Repository) FindActiveByUsername(ctx context.Context, username string) (*User, error) {
	var row userRecord
	result := repo.db.WithContext(ctx).
		Where("username = ? AND is_active = true", username).
		Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if result.Error != nil {
		return nil, translateDatabase(result.Error)
	}
	return userFromRecord(row)
}

// FindActiveByID resolves an internal caller's declared user without JWT parsing.
func (repo *Repository) FindActiveByID(ctx context.Context, id ID) (*User, error) {
	var row userRecord
	result := repo.db.WithContext(ctx).Where("id = ? AND is_active = true", id.String()).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if result.Error != nil {
		return nil, translateDatabase(result.Error)
	}
	return userFromRecord(row)
}

func translateDatabase(err error) error {
	if err == nil {
		return nil
	}
	classified := database.TranslateError(err)
	switch {
	case database.IsCanceled(classified):
		return context.Canceled
	case database.IsDeadline(classified):
		return context.DeadlineExceeded
	case database.IsNotFound(classified):
		return ErrNotFound
	default:
		return classified
	}
}
