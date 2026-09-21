package image

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LockArticleReferences serializes reference changes for one article.
func (repo *Repository) LockArticleReferences(ctx context.Context, articleID int64) error {
	return translateDatabase(repo.db.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(?)", articleID).Error)
}

// LockReferenceTransition locks all old and new image rows in stable order.
func (repo *Repository) LockReferenceTransition(ctx context.Context, oldIDs []ArticleImageID, newKeys []StorageKey) ([]*ArticleImage, error) {
	if len(oldIDs) == 0 && len(newKeys) == 0 {
		return []*ArticleImage{}, nil
	}
	ids := make([]string, len(oldIDs))
	for i, v := range oldIDs {
		ids[i] = v.String()
	}
	keys := make([]string, len(newKeys))
	for i, v := range newKeys {
		keys[i] = v.String()
	}
	query := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("cleanup_claim_token IS NULL")
	switch {
	case len(ids) == 0:
		query = query.Where("storage_key IN ?", keys)
	case len(keys) == 0:
		query = query.Where("id IN ?", ids)
	default:
		query = query.Where("id IN ? OR storage_key IN ?", ids, keys)
	}
	var rows []imageModel
	if err := query.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, translateDatabase(err)
	}
	return mapRows(rows)
}

// FindArticleReferences reads the image identifiers linked to an article.
func (repo *Repository) FindArticleReferences(ctx context.Context, articleID int64) ([]ArticleImageID, error) {
	var rows []referenceModel
	if err := repo.db.WithContext(ctx).Where("article_id = ?", articleID).Order("image_id ASC").Find(&rows).Error; err != nil {
		return nil, translateDatabase(err)
	}
	result := make([]ArticleImageID, 0, len(rows))
	for _, row := range rows {
		id, err := NewArticleImageID(row.ImageID)
		if err != nil {
			return nil, fmt.Errorf("map article image reference: %w", err)
		}
		result = append(result, id)
	}
	return result, nil
}

// ReplaceArticleReferences replaces one article's image links.
func (repo *Repository) ReplaceArticleReferences(ctx context.Context, articleID int64, ids []ArticleImageID, now time.Time) error {
	var existing []referenceModel
	if err := repo.db.WithContext(ctx).Where("article_id = ?", articleID).Order("image_id ASC").Find(&existing).Error; err != nil {
		return translateDatabase(err)
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
		if err := repo.db.WithContext(ctx).Where("article_id = ? AND image_id = ?", articleID, link.ImageID).Delete(&referenceModel{}).Error; err != nil {
			return translateDatabase(err)
		}
	}
	additions := make([]string, 0, len(wanted))
	for id := range wanted {
		additions = append(additions, id)
	}
	sort.Strings(additions)
	for _, id := range additions {
		if err := repo.db.WithContext(ctx).Create(&referenceModel{ArticleID: articleID, ImageID: id, CreatedAt: now.UTC()}).Error; err != nil {
			return translateDatabase(err)
		}
	}
	return nil
}

// CountReferencesForUpdate counts references while locking the image row.
func (repo *Repository) CountReferencesForUpdate(ctx context.Context, id ArticleImageID) (int64, error) {
	var image imageModel
	if err := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id.String()).Take(&image).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, notFound()
		}
		return 0, translateDatabase(err)
	}
	var rows []referenceModel
	if err := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("image_id = ?", id.String()).Order("article_id ASC").Find(&rows).Error; err != nil {
		return 0, translateDatabase(err)
	}
	return int64(len(rows)), nil
}

// CountReferencesForLockedImage counts references after the caller acquired the image lock.
func (repo *Repository) CountReferencesForLockedImage(ctx context.Context, id ArticleImageID) (int64, error) {
	var rows []referenceModel
	if err := repo.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("image_id = ?", id.String()).Order("article_id ASC").Find(&rows).Error; err != nil {
		return 0, translateDatabase(err)
	}
	return int64(len(rows)), nil
}
