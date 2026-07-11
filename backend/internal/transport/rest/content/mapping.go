package content

import (
	"fmt"
	"strconv"
	"strings"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func entityTag(version uint64) string { return fmt.Sprintf("\"%d\"", version) }

func parseEntityTag(value string) (uint64, error) {
	if strings.HasPrefix(value, "W/") || len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		return 0, fmt.Errorf("invalid strong entity tag")
	}
	version, err := strconv.ParseUint(value[1:len(value)-1], 10, 64)
	if err != nil || version == 0 {
		return 0, fmt.Errorf("invalid strong entity tag")
	}
	return version, nil
}

func articleDTO(item module.ArticleResult) generated.Article {
	tags := make([]generated.PositiveID, len(item.TagIDs))
	copy(tags, item.TagIDs)
	updated := item.ModifiedAt
	status := generated.Draft
	switch item.Status {
	case "published":
		status = generated.Published
	case "archived":
		status = generated.Archived
	}
	return generated.Article{ID: item.ID, ArticleTypeID: item.ArticleTypeID, Title: item.Title, Slug: item.Slug, Digest: item.Digest, Content: item.Content, Status: status, TagIds: tags, Version: int64(item.Version), CreatedAt: item.CreatedAt, UpdatedAt: &updated}
}

func articleTypeDTO(item module.ArticleTypeResult) generated.ArticleType {
	updated := item.ModifiedAt
	return generated.ArticleType{ID: item.ID, Name: item.Name, Version: int64(item.Version), CreatedAt: item.CreatedAt, UpdatedAt: &updated}
}

func tagDTO(item module.TagResult) generated.Tag {
	updated := item.ModifiedAt
	return generated.Tag{ID: item.ID, Name: item.Name, Version: int64(item.Version), CreatedAt: item.CreatedAt, UpdatedAt: &updated}
}

func articleSort(value *generated.ListArticlesParamsSort) string {
	if value == nil {
		return "newest"
	}
	switch *value {
	case generated.ListArticlesParamsSortCreatedAt:
		return "oldest"
	case generated.ListArticlesParamsSortTitle:
		return "title"
	case generated.ListArticlesParamsSortMinusTitle:
		return "title_desc"
	default:
		return "newest"
	}
}

func pageValues(page *generated.Page, size *generated.PageSize) (int, int) {
	number, pageSize := 1, 20
	if page != nil {
		number = int(*page)
	}
	if size != nil {
		pageSize = int(*size)
	}
	return number, pageSize
}
