package content

import (
	"time"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
)

func articleTypeDTO(item taxonomy.ArticleTypeResult) (generated.ArticleType, error) {
	// 分类文本和分页元数据同样必须满足对外响应契约。
	version, err := generatedVersion(item.Version)
	if err != nil || taxonomyResponseTextInvalid(item) || item.ID <= 0 || invalidTimes(item.CreatedAt, item.ModifiedAt) || item.Meun < 0 {
		return generated.ArticleType{}, responseMappingError()
	}
	var updated *time.Time
	if !item.ModifiedAt.IsZero() {
		value := item.ModifiedAt
		updated = &value
	}
	return generated.ArticleType{ID: item.ID, Name: item.Name, Image: item.Image, Menu: item.Meun, Version: version, CreatedAt: item.CreatedAt, UpdatedAt: updated}, nil
}

func taxonomyResponseTextInvalid(item taxonomy.ArticleTypeResult) bool {
	return item.Name == "" || len([]rune(item.Name)) > 60
}
