package taxonomy

import (
	"context"
	"strings"
	"time"
)

type articleTypeProjectionRow struct {
	ID        int64      `gorm:"column:id"`
	Name      string     `gorm:"column:name"`
	Image     *string    `gorm:"column:image"`
	Meun      int32      `gorm:"column:meun"`
	Version   int64      `gorm:"column:version"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt *time.Time `gorm:"column:updated_at"`
}

func (repo *repository) listArticleTypes(ctx context.Context, name string, page, size int, sortKey string) ([]articleTypeEntity, int64, error) {
	order, err := taxonomyOrder(sortKey)
	if err != nil {
		return nil, 0, err
	}
	query := repo.db.WithContext(ctx).Table("article_types").Where("is_deleted = false")
	if value := strings.TrimSpace(name); value != "" {
		query = query.Where("name ILIKE ?", "%"+value+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, translateDatabase(err)
	}
	var rows []articleTypeProjectionRow
	columns := "id, name, image, meun, version, created_at, updated_at"
	if err := query.Select(columns).Order(order).Limit(size).Offset((page - 1) * size).Scan(&rows).Error; err != nil {
		return nil, 0, translateDatabase(err)
	}
	result := make([]articleTypeEntity, 0, len(rows))
	for _, row := range rows {
		entity, err := entityFromArticleTypeProjection(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, entity)
	}
	return result, total, nil
}
