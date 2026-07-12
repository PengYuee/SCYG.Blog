package postgres

import (
	"context"
	"errors"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"gorm.io/gorm"
)

type articleTypeRepository struct{ db *gorm.DB }

func (repo *articleTypeRepository) NextID(ctx context.Context) (domain.ArticleTypeID, error) {
	var value int64
	if err := repo.db.WithContext(ctx).Raw(`SELECT nextval(pg_get_serial_sequence('"ArticleType"', 'Id'))`).Scan(&value).Error; err != nil {
		return domain.ArticleTypeID{}, translate(err)
	}
	return domain.NewArticleTypeID(value)
}

func (repo *articleTypeRepository) Find(ctx context.Context, id domain.ArticleTypeID) (*domain.ArticleType, error) {
	var row articleTypeModel
	result := repo.db.WithContext(ctx).Where(`"Id" = ? AND "IsDeleted" = false`, id.Int64()).First(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, notFound("article type")
	}
	if result.Error != nil {
		return nil, translate(result.Error)
	}
	item, err := articleTypeFromModel(row)
	if err != nil {
		return nil, err
	}
	return item, nil
}
func (repo *articleTypeRepository) Save(ctx context.Context, item *domain.ArticleType) error {
	row := articleTypeModel{ID: item.ID().Int64(), Name: item.Name().String(), Image: item.Image(), Meun: int16(item.Meun()), Version: int64(item.Version().Uint64()), CreationTime: item.CreatedAt().UTC(), LastModificationTime: nullableTime(item.ModifiedAt()), DeletionTime: nullableTime(item.DeletedAt()), IsDeleted: !item.DeletedAt().IsZero()}
	if row.Version == 1 {
		result := repo.db.WithContext(ctx).Select("Id", "Name", "Image", "Meun", "Version", "CreationTime", "LastModificationTime", "DeletionTime", "IsDeleted").Create(&row)
		return translate(result.Error)
	}
	if row.IsDeleted {
		var count int64
		if err := repo.db.WithContext(ctx).Model(&articleModel{}).Where(`"ArticleTypeId" = ? AND "IsDeleted" = false`, row.ID).Count(&count).Error; err != nil {
			return translate(err)
		}
		if count > 0 {
			return failedPrecondition()
		}
	}
	expected := row.Version - 1
	result := repo.db.WithContext(ctx).Model(&articleTypeModel{}).Where(`"Id" = ? AND "Version" = ? AND "IsDeleted" = false`, row.ID, expected).Updates(map[string]any{"Name": row.Name, "Image": row.Image, "Meun": row.Meun, "LastModificationTime": row.LastModificationTime, "DeletionTime": row.DeletionTime, "IsDeleted": row.IsDeleted, "Version": gorm.Expr(`"Version" + 1`)})
	if result.Error != nil {
		return translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return classifyMiss(ctx, repo.db, "ArticleType", row.ID, uint64(expected))
	}
	return nil
}
