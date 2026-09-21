package taxonomy

import (
	"context"
	"errors"
	"math"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/persistence"
)

func (repo *repository) nextArticleTypeID(ctx context.Context) (int64, error) {
	var value int64
	if err := repo.db.WithContext(ctx).Raw(`SELECT nextval(pg_get_serial_sequence('article_types', 'id'))`).Scan(&value).Error; err != nil {
		return 0, translateDatabase(err)
	}
	return value, nil
}

func (repo *repository) findArticleType(ctx context.Context, id int64, lock bool) (articleTypeEntity, error) {
	var row articleTypeModel
	query := repo.db.WithContext(ctx).Where("id = ? AND is_deleted = false", id)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	result := query.First(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return articleTypeEntity{}, notFound("article type")
	}
	if result.Error != nil {
		return articleTypeEntity{}, translateDatabase(result.Error)
	}
	return entityFromArticleType(row)
}

func (repo *repository) createArticleType(ctx context.Context, entity articleTypeEntity) error {
	if entity.Meun < 0 || entity.Meun > math.MaxInt16 || entity.Version == 0 || entity.Version > math.MaxInt64 {
		return invalid("article_type")
	}
	row := articleTypeModel{ID: entity.ID, Name: entity.Name, Image: copyImage(entity.Image), Meun: int16(entity.Meun), Version: int64(entity.Version), AuditFields: persistence.NewAuditFields(entity.CreatedAt, entity.Modified, entity.DeletedAt)}
	return translateDatabase(repo.db.WithContext(ctx).Select("id", "name", "image", "meun", "version", "created_at", "updated_at", "deleted_at", "is_deleted").Create(&row).Error)
}

func (repo *repository) updateArticleType(ctx context.Context, entity articleTypeEntity, expected uint64) error {
	if !entity.DeletedAt.IsZero() {
		var count int64
		if err := repo.db.WithContext(ctx).Model(&articleModel{}).Where("article_type_id = ? AND is_deleted = false", entity.ID).Count(&count).Error; err != nil {
			return translateDatabase(err)
		}
		if count > 0 {
			return precondition()
		}
	}
	audit := persistence.NewAuditFields(entity.CreatedAt, entity.Modified, entity.DeletedAt)
	result := repo.db.WithContext(ctx).Model(&articleTypeModel{}).Where("id = ? AND version = ? AND is_deleted = false", entity.ID, expected).Updates(map[string]any{
		"name": entity.Name, "image": entity.Image, "meun": entity.Meun, "updated_at": audit.UpdatedAt, "deleted_at": audit.DeletedAt, "is_deleted": audit.IsDeleted, "version": gorm.Expr("version + 1"),
	})
	if result.Error != nil {
		return translateDatabase(result.Error)
	}
	if result.RowsAffected == 0 {
		return repo.classifyMiss(ctx, "article_types", entity.ID, expected)
	}
	return nil
}
