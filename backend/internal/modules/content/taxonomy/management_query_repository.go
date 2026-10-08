package taxonomy

import "context"

func (repo *Repository) tagsByIDs(ctx context.Context, ids []int64) ([]TagResult, error) {
	if len(ids) == 0 {
		return []TagResult{}, nil
	}
	var rows []tagProjectionRow
	if err := repo.db.WithContext(ctx).Table("tags").Select("id, name, version, created_at, updated_at").Where("id IN ? AND is_deleted = false", ids).Order("id").Scan(&rows).Error; err != nil {
		return nil, translateDatabase(err)
	}
	result := make([]TagResult, 0, len(rows))
	for _, row := range rows {
		entity, err := entityFromTagProjection(row)
		if err != nil {
			return nil, stable(err)
		}
		result = append(result, tagResult(entity))
	}
	return result, nil
}
