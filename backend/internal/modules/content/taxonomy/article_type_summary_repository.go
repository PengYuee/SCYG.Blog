package taxonomy

import (
	"context"

	"gorm.io/gorm/clause"
)

type articleTypeSummaryRow struct {
	ID    int64   `gorm:"column:id"`
	Name  string  `gorm:"column:name"`
	Image *string `gorm:"column:image"`
}

func (repo *repository) articleTypeSummaries(ctx context.Context, ids []int64, lock bool) ([]ArticleTypeSummary, error) {
	var rows []articleTypeSummaryRow
	query := repo.db.WithContext(ctx).Table("article_types").Select("id, name, image").Where("id IN ? AND is_deleted = false", ids).Order("id")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "SHARE"})
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, translateDatabase(err)
	}
	if len(rows) != len(ids) {
		return nil, notFound("article_type")
	}
	items := make([]ArticleTypeSummary, len(rows))
	for index, row := range rows {
		name, err := parseName(row.Name)
		if err != nil {
			return nil, ErrInvalidData
		}
		image, err := parseImage(row.Image)
		if err != nil {
			return nil, ErrInvalidData
		}
		items[index] = ArticleTypeSummary{ID: row.ID, Name: name, Image: image}
	}
	return items, nil
}
