package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type articleImageRepository struct{ db *gorm.DB }

func (repo *articleImageRepository) Save(ctx context.Context, image *domain.ArticleImage) error {
	row := articleImageToModel(image)
	result := repo.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"owner_id", "storage_key", "media_type", "byte_size", "width", "height", "sha256", "status", "created_at", "committed_at", "orphaned_at", "expires_at"})}).Create(&row)
	return translate(result.Error)
}

func (repo *articleImageRepository) Find(ctx context.Context, id domain.ArticleImageID) (*domain.ArticleImage, error) {
	return repo.find(repo.db.WithContext(ctx).Where("id = ?", id.String()))
}

func (repo *articleImageRepository) FindByStorageKey(ctx context.Context, key domain.StorageKey) (*domain.ArticleImage, error) {
	return repo.find(repo.db.WithContext(ctx).Where("storage_key = ?", key.String()))
}

func (repo *articleImageRepository) find(query *gorm.DB) (*domain.ArticleImage, error) {
	var row articleImageModel
	result := query.Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, notFound("article image")
	}
	if result.Error != nil {
		return nil, translate(result.Error)
	}
	image, err := articleImageFromModel(row)
	if err != nil {
		return nil, fmt.Errorf("map article image: %w", err)
	}
	return image, nil
}

func (repo *articleImageRepository) FindOwner(ctx context.Context, id domain.ArticleImageID) (domain.ImageOwnerID, error) {
	var row struct {
		OwnerID string `gorm:"column:owner_id"`
	}
	result := repo.db.WithContext(ctx).Model(&articleImageModel{}).Select("owner_id").Where("id = ?", id.String()).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return domain.ImageOwnerID{}, notFound("article image")
	}
	if result.Error != nil {
		return domain.ImageOwnerID{}, translate(result.Error)
	}
	return domain.NewImageOwnerID(row.OwnerID)
}

func (repo *articleImageRepository) FindForUpdate(ctx context.Context, ids []domain.ArticleImageID) ([]*domain.ArticleImage, error) {
	if len(ids) == 0 {
		return []*domain.ArticleImage{}, nil
	}
	raw := make([]string, len(ids))
	for index, id := range ids {
		raw[index] = id.String()
	}
	sort.Strings(raw)
	var rows []articleImageModel
	if err := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", raw).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, translate(err)
	}
	result := make([]*domain.ArticleImage, 0, len(rows))
	for _, row := range rows {
		image, err := articleImageFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, image)
	}
	return result, nil
}

// FindForUpdateByStorageKeys 按存储键稳定排序并锁定图片元数据行。
func (repo *articleImageRepository) FindForUpdateByStorageKeys(ctx context.Context, keys []domain.StorageKey) ([]*domain.ArticleImage, error) {
	if len(keys) == 0 {
		return []*domain.ArticleImage{}, nil
	}
	raw := make([]string, len(keys))
	for index, key := range keys {
		raw[index] = key.String()
	}
	sort.Strings(raw)
	var rows []articleImageModel
	if err := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("storage_key IN ?", raw).Order("storage_key ASC").Find(&rows).Error; err != nil {
		return nil, translate(err)
	}
	result := make([]*domain.ArticleImage, 0, len(rows))
	for _, row := range rows {
		image, err := articleImageFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, image)
	}
	return result, nil
}

// LockReferenceTransition 将旧引用与新存储键映射到同一 ID 序列并一次性稳定加锁。
func (repo *articleImageRepository) LockReferenceTransition(ctx context.Context, oldIDs []domain.ArticleImageID, newKeys []domain.StorageKey) ([]*domain.ArticleImage, error) {
	if len(oldIDs) == 0 && len(newKeys) == 0 {
		return []*domain.ArticleImage{}, nil
	}
	rawIDs := make([]string, len(oldIDs))
	for index, id := range oldIDs {
		rawIDs[index] = id.String()
	}
	rawKeys := make([]string, len(newKeys))
	for index, key := range newKeys {
		rawKeys[index] = key.String()
	}
	query := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"})
	switch {
	case len(rawIDs) == 0:
		query = query.Where("storage_key IN ?", rawKeys)
	case len(rawKeys) == 0:
		query = query.Where("id IN ?", rawIDs)
	default:
		query = query.Where("id IN ? OR storage_key IN ?", rawIDs, rawKeys)
	}
	var rows []articleImageModel
	if err := query.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, translate(err)
	}
	result := make([]*domain.ArticleImage, 0, len(rows))
	for _, row := range rows {
		image, err := articleImageFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, image)
	}
	return result, nil
}

// FindArticleReferences 返回文章当前引用的图片标识，并保持稳定顺序。
func (repo *articleImageRepository) FindArticleReferences(ctx context.Context, articleID domain.ArticleID) ([]domain.ArticleImageID, error) {
	var rows []articleImageReferenceModel
	if err := repo.db.WithContext(ctx).Where("article_id = ?", articleID.Int64()).Order("image_id ASC").Find(&rows).Error; err != nil {
		return nil, translate(err)
	}
	result := make([]domain.ArticleImageID, 0, len(rows))
	for _, row := range rows {
		id, err := domain.NewArticleImageID(row.ImageID)
		if err != nil {
			return nil, fmt.Errorf("map article image reference: %w", err)
		}
		result = append(result, id)
	}
	return result, nil
}

func (repo *articleImageRepository) ReplaceArticleReferences(ctx context.Context, articleID domain.ArticleID, ids []domain.ArticleImageID, now time.Time) error {
	var existing []articleImageReferenceModel
	if err := repo.db.WithContext(ctx).Where("article_id = ?", articleID.Int64()).Order("image_id ASC").Find(&existing).Error; err != nil {
		return translate(err)
	}
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		wanted[id.String()] = struct{}{}
	}
	for _, link := range existing {
		if _, keep := wanted[link.ImageID]; keep {
			delete(wanted, link.ImageID)
			continue
		}
		if err := repo.db.WithContext(ctx).Where("article_id = ? AND image_id = ?", articleID.Int64(), link.ImageID).Delete(&articleImageReferenceModel{}).Error; err != nil {
			return translate(err)
		}
	}
	additions := make([]string, 0, len(wanted))
	for id := range wanted {
		additions = append(additions, id)
	}
	sort.Strings(additions)
	for _, id := range additions {
		link := articleImageReferenceModel{ArticleID: articleID.Int64(), ImageID: id, CreatedAt: now.UTC()}
		if err := repo.db.WithContext(ctx).Create(&link).Error; err != nil {
			return translate(err)
		}
	}
	return nil
}

func (repo *articleImageRepository) CountReferencesForUpdate(ctx context.Context, id domain.ArticleImageID) (int64, error) {
	var image articleImageModel
	if err := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id.String()).Take(&image).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, notFound("article image")
		}
		return 0, translate(err)
	}
	var rows []articleImageReferenceModel
	err := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("image_id = ?", id.String()).Order("article_id ASC").Find(&rows).Error
	if err != nil {
		return 0, translate(err)
	}
	return int64(len(rows)), nil
}

// CountReferencesForLockedImage 在调用方已持有图片行锁时仅稳定锁定并统计引用行。
func (repo *articleImageRepository) CountReferencesForLockedImage(ctx context.Context, id domain.ArticleImageID) (int64, error) {
	var rows []articleImageReferenceModel
	if err := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("image_id = ?", id.String()).Order("article_id ASC").Find(&rows).Error; err != nil {
		return 0, translate(err)
	}
	return int64(len(rows)), nil
}

func (repo *articleImageRepository) ListExpiredPending(ctx context.Context, cutoff time.Time, limit int) ([]*domain.ArticleImage, error) {
	return repo.listExpired(ctx, "pending", "expires_at", cutoff, limit)
}

func (repo *articleImageRepository) ListExpiredOrphaned(ctx context.Context, cutoff time.Time, limit int) ([]*domain.ArticleImage, error) {
	return repo.listExpired(ctx, "orphaned", "expires_at", cutoff, limit)
}

func (repo *articleImageRepository) listExpired(ctx context.Context, status, column string, cutoff time.Time, limit int) ([]*domain.ArticleImage, error) {
	if limit < 1 {
		return []*domain.ArticleImage{}, nil
	}
	var rows []articleImageModel
	query := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status = ? AND "+column+" <= ?", status, cutoff.UTC()).Order(column + " ASC, id ASC").Limit(limit)
	if err := query.Find(&rows).Error; err != nil {
		return nil, translate(err)
	}
	result := make([]*domain.ArticleImage, 0, len(rows))
	for _, row := range rows {
		image, err := articleImageFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, image)
	}
	return result, nil
}

func (repo *articleImageRepository) DeleteMetadata(ctx context.Context, id domain.ArticleImageID) error {
	return translate(repo.db.WithContext(ctx).Where("id = ?", id.String()).Delete(&articleImageModel{}).Error)
}
