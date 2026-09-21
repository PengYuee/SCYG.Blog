package taxonomy

import (
	"context"
	"strings"
	"time"
)

type tagProjectionRow struct {
	ID        int64      `gorm:"column:id"`
	Name      string     `gorm:"column:name"`
	Version   int64      `gorm:"column:version"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt *time.Time `gorm:"column:updated_at"`
}

func (repo *repository) listTags(ctx context.Context, name string, page, size int, sortKey string) ([]tagEntity, int64, error) {
	order, err := taxonomyOrder(sortKey)
	if err != nil {
		return nil, 0, err
	}
	query := repo.db.WithContext(ctx).Table("tags").Where("is_deleted = false")
	if value := strings.TrimSpace(name); value != "" {
		query = query.Where("name ILIKE ?", "%"+value+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, translateDatabase(err)
	}
	var rows []tagProjectionRow
	columns := "id, name, version, created_at, updated_at"
	if err := query.Select(columns).Order(order).Limit(size).Offset((page - 1) * size).Scan(&rows).Error; err != nil {
		return nil, 0, translateDatabase(err)
	}
	result := make([]tagEntity, 0, len(rows))
	for _, row := range rows {
		entity, err := entityFromTagProjection(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, entity)
	}
	return result, total, nil
}
