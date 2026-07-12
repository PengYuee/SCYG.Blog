package postgres

import (
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func articleTypeFromModel(row articleTypeModel) (*domain.ArticleType, error) {
	id, err := domain.NewArticleTypeID(row.ID)
	if err != nil {
		return nil, err
	}
	name, err := domain.NewName(row.Name)
	if err != nil {
		return nil, err
	}
	version, err := domain.NewVersion(uint64(row.Version))
	if err != nil {
		return nil, err
	}
	return domain.ReconstituteArticleType(domain.ArticleTypeState{TaxonomyState: domain.TaxonomyState[domain.ArticleTypeID]{ID: id, Name: name, Version: version, CreatedAt: row.CreationTime.UTC(), ModifiedAt: timeValue(row.LastModificationTime, row.CreationTime), DeletedAt: timeValue(row.DeletionTime, time.Time{})}, Image: row.Image, Meun: int32(row.Meun)})
}
