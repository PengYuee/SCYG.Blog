package postgres

import (
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func tagFromModel(row tagModel) (*domain.Tag, error) {
	id, err := domain.NewTagID(row.ID)
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
	return domain.ReconstituteTag(domain.TaxonomyState[domain.TagID]{ID: id, Name: name, Version: version, CreatedAt: row.CreationTime.UTC(), ModifiedAt: timeValue(row.LastModificationTime, row.CreationTime), DeletedAt: timeValue(row.DeletionTime, time.Time{})})
}
