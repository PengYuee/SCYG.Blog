package content

import (
	"time"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
)

func tagDTO(item taxonomy.TagResult) (generated.Tag, error) {
	version, err := generatedVersion(item.Version)
	if err != nil || taxonomyTagResponseTextInvalid(item) || item.ID <= 0 || invalidTimes(item.CreatedAt, item.ModifiedAt) {
		return generated.Tag{}, responseMappingError()
	}
	var updated *time.Time
	if !item.ModifiedAt.IsZero() {
		value := item.ModifiedAt
		updated = &value
	}
	return generated.Tag{ID: item.ID, Name: item.Name, Version: version, CreatedAt: item.CreatedAt, UpdatedAt: updated}, nil
}

func taxonomyTagResponseTextInvalid(item taxonomy.TagResult) bool {
	return item.Name == "" || len([]rune(item.Name)) > 60
}
