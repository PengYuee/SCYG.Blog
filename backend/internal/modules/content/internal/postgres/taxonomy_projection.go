package postgres

import (
	"context"
	"strings"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

type taxonomyProjectionRow struct {
	ID      int64  `gorm:"column:Id"`
	Name    string `gorm:"column:Name"`
	Version int64  `gorm:"column:Version"`
}

func (read *ReadModel) ListArticleTypes(ctx context.Context, name string) ([]application.ArticleTypeView, error) {
	query := read.db.WithContext(ctx).Table(`"ArticleType"`).Select(`"Id", "Name", "Version"`).Where(`"IsDeleted" = false`)
	if value := strings.TrimSpace(name); value != "" {
		query = query.Where(`"Name" ILIKE ?`, "%"+value+"%")
	}
	var rows []taxonomyProjectionRow
	if err := query.Order(`"Name" ASC, "Id" ASC`).Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}
	result := make([]application.ArticleTypeView, 0, len(rows))
	for _, row := range rows {
		id, err := domain.NewArticleTypeID(row.ID)
		if err != nil {
			return nil, err
		}
		parsedName, err := domain.NewName(row.Name)
		if err != nil {
			return nil, err
		}
		version, err := domain.NewVersion(uint64(row.Version))
		if err != nil {
			return nil, err
		}
		result = append(result, application.ArticleTypeView{ID: id, Name: parsedName, Version: version})
	}
	return result, nil
}
func (read *ReadModel) ListTags(ctx context.Context, name string) ([]application.TagView, error) {
	query := read.db.WithContext(ctx).Table(`"Tag"`).Select(`"Id", "Name", "Version"`).Where(`"IsDeleted" = false`)
	if value := strings.TrimSpace(name); value != "" {
		query = query.Where(`"Name" ILIKE ?`, "%"+value+"%")
	}
	var rows []taxonomyProjectionRow
	if err := query.Order(`"Name" ASC, "Id" ASC`).Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}
	result := make([]application.TagView, 0, len(rows))
	for _, row := range rows {
		id, err := domain.NewTagID(row.ID)
		if err != nil {
			return nil, err
		}
		parsedName, err := domain.NewName(row.Name)
		if err != nil {
			return nil, err
		}
		version, err := domain.NewVersion(uint64(row.Version))
		if err != nil {
			return nil, err
		}
		result = append(result, application.TagView{ID: id, Name: parsedName, Version: version})
	}
	return result, nil
}

var _ application.TaxonomyReadModel = (*ReadModel)(nil)
