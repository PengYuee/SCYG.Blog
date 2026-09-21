package taxonomy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
)

// Repository is the concrete taxonomy persistence boundary.
type Repository struct{ db *gorm.DB }

// NewRepository creates a taxonomy repository over one GORM handle.
func NewRepository(db *gorm.DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("taxonomy database is nil")
	}
	return &Repository{db: db}, nil
}

func (repo *Repository) internal() *repository { return newRepository(repo.db) }

type repository struct{ db *gorm.DB }

func newRepository(db *gorm.DB) *repository { return &repository{db: db} }

func (repo *repository) classifyMiss(ctx context.Context, table string, id int64, expected uint64) error {
	var row struct {
		Version   uint64 `gorm:"column:version"`
		IsDeleted bool   `gorm:"column:is_deleted"`
	}
	result := repo.db.WithContext(ctx).Table(table).Select("version, is_deleted").Where("id = ?", id).First(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return notFound(strings.ToLower(table))
	}
	if result.Error != nil {
		return translateDatabase(result.Error)
	}
	if row.IsDeleted {
		return precondition()
	}
	return stale(expected, row.Version)
}

func translateDatabase(err error) error {
	if err == nil {
		return nil
	}
	translated := database.TranslateError(err)
	switch {
	case database.IsUnique(translated):
		return conflict()
	case database.IsForeignKey(translated):
		return precondition()
	case database.IsNotFound(translated):
		return notFound("resource")
	case database.IsCanceled(translated):
		return context.Canceled
	case database.IsDeadline(translated):
		return context.DeadlineExceeded
	default:
		return internalWithCause(fmt.Errorf("%w", translated))
	}
}

func internalWithCause(err error) error {
	return &Error{Code: CodeInternal, Cause: errors.Join(ErrPersistence, err)}
}
