package taxonomy

import (
	"context"
	"errors"
	"math"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/persistence"
)

func (repo *repository) nextTagID(ctx context.Context) (int64, error) {
	var value int64
	if err := repo.db.WithContext(ctx).Raw(`SELECT nextval(pg_get_serial_sequence('tags', 'id'))`).Scan(&value).Error; err != nil {
		return 0, translateDatabase(err)
	}
	return value, nil
}

func (repo *repository) findTag(ctx context.Context, id int64, lock bool) (tagEntity, error) {
	var row tagModel
	query := repo.db.WithContext(ctx).Where("id = ? AND is_deleted = false", id)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	result := query.First(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return tagEntity{}, notFound("tag")
	}
	if result.Error != nil {
		return tagEntity{}, translateDatabase(result.Error)
	}
	return entityFromTag(row)
}

func (repo *repository) createTag(ctx context.Context, entity tagEntity) error {
	if entity.Version == 0 || entity.Version > math.MaxInt64 {
		return invalid("tag")
	}
	row := tagModel{ID: entity.ID, Name: entity.Name, Version: int64(entity.Version), AuditFields: persistence.NewAuditFields(entity.CreatedAt, entity.Modified, entity.DeletedAt)}
	return translateDatabase(repo.db.WithContext(ctx).Select("id", "name", "version", "created_at", "updated_at", "deleted_at", "is_deleted").Create(&row).Error)
}

func (repo *repository) updateTag(ctx context.Context, entity tagEntity, expected uint64) error {
	if !entity.DeletedAt.IsZero() {
		var count int64
		if err := repo.db.WithContext(ctx).Model(&tagArticleModel{}).Joins("JOIN articles ON articles.id = article_tags.article_id").Where("article_tags.tag_id = ? AND articles.is_deleted = false", entity.ID).Count(&count).Error; err != nil {
			return translateDatabase(err)
		}
		if count > 0 {
			return precondition()
		}
	}
	audit := persistence.NewAuditFields(entity.CreatedAt, entity.Modified, entity.DeletedAt)
	result := repo.db.WithContext(ctx).Model(&tagModel{}).Where("id = ? AND version = ? AND is_deleted = false", entity.ID, expected).Updates(map[string]any{
		"name": entity.Name, "updated_at": audit.UpdatedAt, "deleted_at": audit.DeletedAt, "is_deleted": audit.IsDeleted, "version": gorm.Expr("version + 1"),
	})
	if result.Error != nil {
		return translateDatabase(result.Error)
	}
	if result.RowsAffected == 0 {
		return repo.classifyMiss(ctx, "tags", entity.ID, expected)
	}
	return nil
}
