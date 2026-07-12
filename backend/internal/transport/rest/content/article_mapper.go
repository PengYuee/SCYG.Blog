package content

import (
	"time"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func articleDTO(item module.ArticleResult) (generated.Article, error) {
	// 响应离开应用边界前必须重新校验，防止持久化异常数据突破 OpenAPI 契约。
	version, err := generatedVersion(item.Version)
	if err != nil || module.ValidateArticleResponseText(item) != nil || item.ID <= 0 || item.ArticleTypeID <= 0 || invalidTimes(item.CreatedAt, item.ModifiedAt) || item.Support < 0 || item.Comment < 0 || item.Visited < 0 {
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

func statusName(status generated.ArticleStatus) string {
	switch status {
	case generated.Published:
		return "published"
	case generated.Archived:
		return "archived"
	default:
		return "draft"
	}
}
