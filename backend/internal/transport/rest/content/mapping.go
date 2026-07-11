package content

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func entityTag(version uint64) (string, error) {
	if version == 0 || version > math.MaxInt64 {
		return "", responseMappingError()
	}
	return fmt.Sprintf("\"%d\"", version), nil
}

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

func articleDTO(item module.ArticleResult) (generated.Article, error) {
	version, err := generatedVersion(item.Version)
	if err != nil || item.ID <= 0 || item.ArticleTypeID <= 0 || item.CreatedAt.IsZero() || item.Support < 0 || item.Comment < 0 || item.Visited < 0 {
		return generated.Article{}, responseMappingError()
	}
	tags := make([]generated.PositiveID, len(item.TagIDs))
	for index, id := range item.TagIDs {
		if id <= 0 {
			return generated.Article{}, responseMappingError()
		}
		tags[index] = id
	}
	var updated *time.Time
	if !item.ModifiedAt.IsZero() {
		value := item.ModifiedAt
		updated = &value
	}
	var status generated.ArticleStatus
	switch item.Status {
	case "draft":
		status = generated.Draft
	case "published":
		status = generated.Published
	case "archived":
		status = generated.Archived
	default:
		return generated.Article{}, responseMappingError()
	}
	return generated.Article{ID: item.ID, ArticleTypeID: item.ArticleTypeID, Title: item.Title, Slug: item.Slug, Digest: item.Digest, Content: item.Content, Status: status, TagIds: tags, Support: item.Support, Comment: item.Comment, Visited: item.Visited, Version: version, CreatedAt: item.CreatedAt, UpdatedAt: updated}, nil
}

func articleTypeDTO(item module.ArticleTypeResult) (generated.ArticleType, error) {
	version, err := generatedVersion(item.Version)
	if err != nil || item.ID <= 0 || item.CreatedAt.IsZero() || item.Meun < 0 {
		return generated.ArticleType{}, responseMappingError()
	}
	var updated *time.Time
	if !item.ModifiedAt.IsZero() {
		value := item.ModifiedAt
		updated = &value
	}
	return generated.ArticleType{ID: item.ID, Name: item.Name, Image: item.Image, Meun: item.Meun, Version: version, CreatedAt: item.CreatedAt, UpdatedAt: updated}, nil
}

func tagDTO(item module.TagResult) (generated.Tag, error) {
	version, err := generatedVersion(item.Version)
	if err != nil || item.ID <= 0 || item.CreatedAt.IsZero() {
		return generated.Tag{}, responseMappingError()
	}
	var updated *time.Time
	if !item.ModifiedAt.IsZero() {
		value := item.ModifiedAt
		updated = &value
	}
	return generated.Tag{ID: item.ID, Name: item.Name, Version: version, CreatedAt: item.CreatedAt, UpdatedAt: updated}, nil
}

func generatedVersion(version uint64) (generated.Version, error) {
	if version == 0 || version > math.MaxInt64 {
		return 0, responseMappingError()
	}
	return int64(version), nil
}

func responseMappingError() error {
	return &module.ApplicationError{Code: module.CodeInternal, Kind: module.KindInternal, Cause: module.ErrPersistence}
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
