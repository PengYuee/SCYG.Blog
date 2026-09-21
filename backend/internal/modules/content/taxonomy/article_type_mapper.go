package taxonomy

import (
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/persistence"
)

type articleTypeEntity struct {
	ID        int64
	Name      string
	Image     *string
	Meun      int32
	Version   uint64
	CreatedAt time.Time
	Modified  time.Time
	DeletedAt time.Time
}

func entityFromArticleType(row articleTypeModel) (articleTypeEntity, error) {
	if err := row.Validate(); err != nil {
		return articleTypeEntity{}, ErrInvalidData
	}
	if err := parseID(row.ID, "article_type_id"); err != nil || row.Version <= 0 || row.CreatedAt.IsZero() {
		return articleTypeEntity{}, ErrInvalidData
	}
	name, err := parseName(row.Name)
	if err != nil {
		return articleTypeEntity{}, ErrInvalidData
	}
	if _, err := parseImage(row.Image); err != nil || row.Meun < 0 {
		return articleTypeEntity{}, ErrInvalidData
	}
	return articleTypeEntity{ID: row.ID, Name: name, Image: copyImage(row.Image), Meun: int32(row.Meun), Version: uint64(row.Version), CreatedAt: row.CreatedAt.UTC(), Modified: row.EffectiveUpdatedAt(), DeletedAt: nullableTimeValue(row.DeletedAt)}, nil
}

func entityFromArticleTypeProjection(row articleTypeProjectionRow) (articleTypeEntity, error) {
	if err := parseID(row.ID, "article_type_id"); err != nil || row.Version <= 0 || row.CreatedAt.IsZero() {
		return articleTypeEntity{}, ErrInvalidData
	}
	name, err := parseName(row.Name)
	if err != nil {
		return articleTypeEntity{}, ErrInvalidData
	}
	if _, err := parseImage(row.Image); err != nil || row.Meun < 0 {
		return articleTypeEntity{}, ErrInvalidData
	}
	return articleTypeEntity{ID: row.ID, Name: name, Image: copyImage(row.Image), Meun: row.Meun, Version: uint64(row.Version), CreatedAt: row.CreatedAt.UTC(), Modified: persistence.EffectiveUpdatedAt(row.CreatedAt, row.UpdatedAt)}, nil
}

func articleTypeResult(entity articleTypeEntity) ArticleTypeResult {
	return ArticleTypeResult{ID: entity.ID, Name: entity.Name, Image: copyImage(entity.Image), Meun: entity.Meun, Version: entity.Version, CreatedAt: entity.CreatedAt, ModifiedAt: entity.Modified}
}
