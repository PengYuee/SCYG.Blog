package postgres

import (
	"context"
	"strings"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func (read *ReadModel) FindArticleType(ctx context.Context, id domain.ArticleTypeID) (application.ArticleTypeView, error) {
	return read.articleType(ctx, id)
}

func (read *ReadModel) ListArticleTypes(ctx context.Context, name string) ([]application.ArticleTypeView, error) {
	query := read.db.WithContext(ctx).Table(`"ArticleType"`).Select(`"Id", "Name", "Image", "Meun", "Version", "CreationTime", "LastModificationTime"`).Where(`"IsDeleted" = false`)
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
		result = append(result, application.ArticleTypeView{ID: id, Name: parsedName, Image: row.Image, Meun: row.Meun, Version: version, CreatedAt: row.CreationTime.UTC(), ModifiedAt: timeValue(row.LastModificationTime, row.CreationTime)})
	}
	return result, nil
}
func (read *ReadModel) articleType(ctx context.Context, id domain.ArticleTypeID) (application.ArticleTypeView, error) {
	items, err := read.ListArticleTypes(ctx, "")
	if err != nil {
		return application.ArticleTypeView{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return application.ArticleTypeView{}, notFound("article type")
}

var _ application.TaxonomyReadModel = (*ReadModel)(nil)
