package content

import (
	"time"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func tagDTO(item module.TagResult) (generated.Tag, error) {
	version, err := generatedVersion(item.Version)
	if err != nil || module.ValidateTagResponseText(item) != nil || item.ID <= 0 || invalidTimes(item.CreatedAt, item.ModifiedAt) {
		return generated.Tag{}, responseMappingError()
	}
	var updated *time.Time
	if !item.ModifiedAt.IsZero() {
		value := item.ModifiedAt
		updated = &value
	}
	return generated.Tag{ID: item.ID, Name: item.Name, Version: version, CreatedAt: item.CreatedAt, UpdatedAt: updated}, nil
}

func tagSort(value *generated.ListTagsParamsSort) string {
	if value == nil {
		return "title"
	}
	return string(*value)
}
