package image

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
)

// Repository persists image metadata and maps database failures to stable errors.
type Repository struct{ db *gorm.DB }

// NewRepository constructs an image repository.
func NewRepository(db *gorm.DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("image repository database is nil")
	}
	return &Repository{db: db}, nil
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
	case database.IsUnique(classified):
		return &Error{Code: CodeConflict, Cause: classified}
	default:
		return internal(classified)
	}
}

// Create persists newly staged image metadata and rejects duplicate identifiers.
func (repo *Repository) Create(ctx context.Context, value *ArticleImage) error {
	row := imageToModel(value)
	return translateDatabase(repo.db.WithContext(ctx).Select("id", "owner_id", "storage_key", "media_type", "byte_size", "width", "height", "sha256", "status", "created_at", "committed_at", "orphaned_at", "expires_at").Create(&row).Error)
}

// UpdateLifecycle persists only mutable image lifecycle fields.
func (repo *Repository) UpdateLifecycle(ctx context.Context, value *ArticleImage) error {
	row := imageToModel(value)
	result := repo.db.WithContext(ctx).Model(&imageModel{}).Where("id = ? AND cleanup_claim_token IS NULL", row.ID).Updates(map[string]any{
		"status":       row.Status,
		"committed_at": row.CommittedAt,
		"orphaned_at":  row.OrphanedAt,
		"expires_at":   row.ExpiresAt,
	})
	if result.Error != nil {
		return translateDatabase(result.Error)
	}
	if result.RowsAffected == 0 {
		return notFound()
	}
	return nil
}

// DeleteMetadata removes still-pending metadata after an upload compensation.
func (repo *Repository) DeleteMetadata(ctx context.Context, id ArticleImageID) error {
	return translateDatabase(repo.db.WithContext(ctx).Where("id = ? AND status = ? AND cleanup_claim_token IS NULL", id.String(), string(ArticleImageStatusPending)).Delete(&imageModel{}).Error)
}

// Find loads an image by identifier.
func (repo *Repository) Find(ctx context.Context, id ArticleImageID) (*ArticleImage, error) {
	return repo.find(repo.db.WithContext(ctx).Where("id = ?", id.String()))
}

// FindByStorageKey loads an image by its storage key.
func (repo *Repository) FindByStorageKey(ctx context.Context, key StorageKey) (*ArticleImage, error) {
	return repo.find(repo.db.WithContext(ctx).Where("storage_key = ?", key.String()))
}

func (repo *Repository) find(query *gorm.DB) (*ArticleImage, error) {
	var row imageModel
	result := query.Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, notFound()
	}
	if result.Error != nil {
		return nil, translateDatabase(result.Error)
	}
	value, err := imageFromModel(row)
	if err != nil {
		return nil, fmt.Errorf("map article image: %w", err)
	}
	return value, nil
}

// FindOwner reads the owner of one image.
func (repo *Repository) FindOwner(ctx context.Context, id ArticleImageID) (ImageOwnerID, error) {
	var row imageOwnerProjectionRow
	result := repo.db.WithContext(ctx).Model(&imageModel{}).Select("owner_id").Where("id = ?", id.String()).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return ImageOwnerID{}, notFound()
	}
	if result.Error != nil {
		return ImageOwnerID{}, translateDatabase(result.Error)
	}
	return NewImageOwnerID(row.OwnerID)
}

// FindForUpdate locks and loads unclaimed images by identifier.
func (repo *Repository) FindForUpdate(ctx context.Context, ids []ArticleImageID) ([]*ArticleImage, error) {
	if len(ids) == 0 {
		return []*ArticleImage{}, nil
	}
	raw := make([]string, len(ids))
	for i, id := range ids {
		raw[i] = id.String()
	}
	sort.Strings(raw)
	var rows []imageModel
	if err := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ? AND cleanup_claim_token IS NULL", raw).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, translateDatabase(err)
	}
	return mapRows(rows)
}

// FindForUpdateByStorageKeys locks and loads unclaimed images by storage key.
func (repo *Repository) FindForUpdateByStorageKeys(ctx context.Context, keys []StorageKey) ([]*ArticleImage, error) {
	if len(keys) == 0 {
		return []*ArticleImage{}, nil
	}
	raw := make([]string, len(keys))
	for i, key := range keys {
		raw[i] = key.String()
	}
	sort.Strings(raw)
	var rows []imageModel
	if err := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("storage_key IN ? AND cleanup_claim_token IS NULL", raw).Order("storage_key ASC").Find(&rows).Error; err != nil {
		return nil, translateDatabase(err)
	}
	return mapRows(rows)
}
