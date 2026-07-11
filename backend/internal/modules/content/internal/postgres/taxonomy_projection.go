package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

type taxonomyProjectionRow struct {
	ID                   int64      `gorm:"column:Id"`
	Name                 string     `gorm:"column:Name"`
	Version              int64      `gorm:"column:Version"`
	CreationTime         time.Time  `gorm:"column:CreationTime"`
	LastModificationTime *time.Time `gorm:"column:LastModificationTime"`
}

func (read *ReadModel) FindArticleType(ctx context.Context, id domain.ArticleTypeID) (application.ArticleTypeView, error) {
	return read.articleType(ctx, id)
}

func (read *ReadModel) FindTag(ctx context.Context, id domain.TagID) (application.TagView, error) {
	return read.tag(ctx, id)
}

func (read *ReadModel) ListArticleTypes(ctx context.Context, name string) ([]application.ArticleTypeView, error) {
	query := read.db.WithContext(ctx).Table(`"ArticleType"`).Select(`"Id", "Name", "Version", "CreationTime", "LastModificationTime"`).Where(`"IsDeleted" = false`)
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
		result = append(result, application.ArticleTypeView{ID: id, Name: parsedName, Version: version, CreatedAt: row.CreationTime.UTC(), ModifiedAt: timeValue(row.LastModificationTime, row.CreationTime)})
	}
	return result, nil
}
func (read *ReadModel) ListTags(ctx context.Context, name string) ([]application.TagView, error) {
	query := read.db.WithContext(ctx).Table(`"Tag"`).Select(`"Id", "Name", "Version", "CreationTime", "LastModificationTime"`).Where(`"IsDeleted" = false`)
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
		result = append(result, application.TagView{ID: id, Name: parsedName, Version: version, CreatedAt: row.CreationTime.UTC(), ModifiedAt: timeValue(row.LastModificationTime, row.CreationTime)})
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

func (read *ReadModel) tag(ctx context.Context, id domain.TagID) (application.TagView, error) {
	items, err := read.ListTags(ctx, "")
	if err != nil {
		return application.TagView{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return application.TagView{}, notFound("tag")
}

var _ application.TaxonomyReadModel = (*ReadModel)(nil)
