package taxonomy

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

type publicArticleTypeProjectionRow struct {
	ID           int64   `gorm:"column:id"`
	Name         string  `gorm:"column:name"`
	Image        *string `gorm:"column:image"`
	Version      int64   `gorm:"column:version"`
	ArticleCount int64   `gorm:"column:article_count"`
}

type publicTagProjectionRow struct {
	ID           int64  `gorm:"column:id"`
	Name         string `gorm:"column:name"`
	Version      int64  `gorm:"column:version"`
	ArticleCount int64  `gorm:"column:article_count"`
}

func (repo *repository) listPublicArticleTypes(ctx context.Context, name string, page, size int) ([]publicArticleTypeProjectionRow, int64, error) {
	query := repo.publicArticleTypeQuery(ctx, name)
	var total int64
	if err := query.Session(&gorm.Session{}).Distinct("at.id").Count(&total).Error; err != nil {
		return nil, 0, translateDatabase(err)
	}
	var rows []publicArticleTypeProjectionRow
	if err := query.Select("at.id, at.name, at.image, at.version, COUNT(a.id) AS article_count").Group("at.id, at.name, at.image, at.version").Order("article_count DESC, at.id ASC").Limit(size).Offset((page - 1) * size).Scan(&rows).Error; err != nil {
		return nil, 0, translateDatabase(err)
	}
	return rows, total, nil
}

func (repo *repository) findPublicArticleType(ctx context.Context, id int64) (publicArticleTypeProjectionRow, error) {
	var row publicArticleTypeProjectionRow
	result := repo.publicArticleTypeQuery(ctx, "").Where("at.id = ?", id).Select("at.id, at.name, at.image, at.version, COUNT(a.id) AS article_count").Group("at.id, at.name, at.image, at.version").Take(&row)
	if result.Error != nil {
		if isRecordNotFound(result.Error) {
			return row, notFound("article type")
		}
		return row, translateDatabase(result.Error)
	}
	return row, nil
}

func (repo *repository) publicArticleTypeQuery(ctx context.Context, name string) *gorm.DB {
	query := repo.db.WithContext(ctx).Table("article_types AS at").Joins("JOIN articles AS a ON a.article_type_id = at.id AND a.status = 2 AND a.is_deleted = false").Where("at.is_deleted = false")
	if value := strings.TrimSpace(name); value != "" {
		query = query.Where("at.name ILIKE ?", "%"+value+"%")
	}
	return query
}

func (repo *repository) listPublicTags(ctx context.Context, name string, page, size int) ([]publicTagProjectionRow, int64, error) {
	query := repo.publicTagQuery(ctx, name)
	var total int64
	if err := query.Session(&gorm.Session{}).Distinct("t.id").Count(&total).Error; err != nil {
		return nil, 0, translateDatabase(err)
	}
	var rows []publicTagProjectionRow
	if err := query.Select("t.id, t.name, t.version, COUNT(a.id) AS article_count").Group("t.id, t.name, t.version").Order("article_count DESC, t.id ASC").Limit(size).Offset((page - 1) * size).Scan(&rows).Error; err != nil {
		return nil, 0, translateDatabase(err)
	}
	return rows, total, nil
}

func (repo *repository) findPublicTag(ctx context.Context, id int64) (publicTagProjectionRow, error) {
	var row publicTagProjectionRow
	result := repo.publicTagQuery(ctx, "").Where("t.id = ?", id).Select("t.id, t.name, t.version, COUNT(a.id) AS article_count").Group("t.id, t.name, t.version").Take(&row)
	if result.Error != nil {
		if isRecordNotFound(result.Error) {
			return row, notFound("tag")
		}
		return row, translateDatabase(result.Error)
	}
	return row, nil
}

func (repo *repository) publicTagQuery(ctx context.Context, name string) *gorm.DB {
	query := repo.db.WithContext(ctx).Table("tags AS t").Joins("JOIN article_tags AS at ON at.tag_id = t.id").Joins("JOIN articles AS a ON a.id = at.article_id AND a.status = 2 AND a.is_deleted = false").Where("t.is_deleted = false")
	if value := strings.TrimSpace(name); value != "" {
		query = query.Where("t.name ILIKE ?", "%"+value+"%")
	}
	return query
}

func publicArticleTypeResult(row publicArticleTypeProjectionRow) (PublicArticleTypeResult, error) {
	name, err := parseName(row.Name)
	if row.ID <= 0 || row.Version <= 0 || row.ArticleCount <= 0 || err != nil {
		return PublicArticleTypeResult{}, ErrInvalidData
	}
	image, err := parseImage(row.Image)
	if err != nil {
		return PublicArticleTypeResult{}, ErrInvalidData
	}
	return PublicArticleTypeResult{ID: row.ID, Name: name, Image: image, ArticleCount: row.ArticleCount, Version: uint64(row.Version)}, nil
}

func publicTagResult(row publicTagProjectionRow) (PublicTagResult, error) {
	name, err := parseName(row.Name)
	if row.ID <= 0 || row.Version <= 0 || row.ArticleCount <= 0 || err != nil {
		return PublicTagResult{}, ErrInvalidData
	}
	return PublicTagResult{ID: row.ID, Name: name, ArticleCount: row.ArticleCount, Version: uint64(row.Version)}, nil
}

func isRecordNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
