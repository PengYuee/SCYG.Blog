package postgres

import (
	"context"
	"errors"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"gorm.io/gorm"
)

type tagRepository struct{ db *gorm.DB }

func (repo *tagRepository) NextID(ctx context.Context) (domain.TagID, error) {
	var value int64
	if err := repo.db.WithContext(ctx).Raw(`SELECT nextval(pg_get_serial_sequence('"Tag"', 'Id'))`).Scan(&value).Error; err != nil {
		return domain.TagID{}, translate(err)
	}
	return domain.NewTagID(value)
}

func (repo *tagRepository) Find(ctx context.Context, id domain.TagID) (*domain.Tag, error) {
	var row tagModel
	result := repo.db.WithContext(ctx).Where(`"Id" = ? AND "IsDeleted" = false`, id.Int64()).First(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, notFound("tag")
	}
	if result.Error != nil {
		return nil, translate(result.Error)
	}
	tag, err := tagFromModel(row)
	if err != nil {
		return nil, err
	}
	return tag, nil
}
func (repo *tagRepository) Save(ctx context.Context, tag *domain.Tag) error {
	row := tagModel{ID: tag.ID().Int64(), Name: tag.Name().String(), Version: int64(tag.Version().Uint64()), CreationTime: tag.CreatedAt().UTC(), LastModificationTime: nullableTime(tag.ModifiedAt()), DeletionTime: nullableTime(tag.DeletedAt()), IsDeleted: !tag.DeletedAt().IsZero()}
	if row.Version == 1 {
		result := repo.db.WithContext(ctx).Select("Id", "Name", "Version", "CreationTime", "LastModificationTime", "DeletionTime", "IsDeleted").Create(&row)
		return translate(result.Error)
	}
	expected := row.Version - 1
	result := repo.db.WithContext(ctx).Model(&tagModel{}).Where(`"Id" = ? AND "Version" = ? AND "IsDeleted" = false`, row.ID, expected).Updates(map[string]any{"Name": row.Name, "LastModificationTime": row.LastModificationTime, "DeletionTime": row.DeletionTime, "IsDeleted": row.IsDeleted, "Version": gorm.Expr(`"Version" + 1`)})
	if result.Error != nil {
		return translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return classifyMiss(ctx, repo.db, "Tag", row.ID, uint64(expected))
	}
	return nil
}
