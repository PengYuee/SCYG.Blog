package content

import (
	"time"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
)

func articleDTO(item article.Result) (generated.Article, error) {
	// 响应离开应用边界前必须重新校验，防止持久化异常数据突破 OpenAPI 契约。
	version, err := generatedVersion(item.Version)
	if err != nil || articleResponseTextInvalid(item) || item.ID <= 0 || item.ArticleTypeID <= 0 || invalidTimes(item.CreatedAt, item.ModifiedAt) || item.Support < 0 || item.Comment < 0 || item.Visited < 0 {
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

func articleResponseTextInvalid(item article.Result) bool {
	title, titleErr := article.NewTitle(item.Title)
	slug, slugErr := article.NewSlug(item.Slug)
	digest, digestErr := article.NewDigest(item.Digest)
	body, bodyErr := article.NewContent(item.Content)
	return titleErr != nil || slugErr != nil || digestErr != nil || bodyErr != nil || title.String() != item.Title || slug.String() != item.Slug || digest.String() != item.Digest || body.String() != item.Content
}

func articleSort(value *generated.ListArticlesParamsSort) string {
	if value == nil {
		return "newest"
	}
	switch *value {
	case generated.ListArticlesParamsSortCreatedAt:
		return "oldest"
	case generated.ListArticlesParamsSortMinusCreatedAt:
		return "newest"
	case generated.ListArticlesParamsSortTitle:
		return "title"
	case generated.ListArticlesParamsSortMinusTitle:
		return "title_desc"
	case generated.ListArticlesParamsSortUpdatedAt, generated.ListArticlesParamsSortMinusUpdatedAt:
		return "newest"
	default:
		return "newest"
	}
}

func manageArticleSort(value *generated.ListManageArticlesParamsSort) string {
	if value == nil {
		return "newest"
	}
	switch string(*value) {
	case "created_at":
		return "oldest"
	case "-created_at":
		return "newest"
	case "title":
		return "title"
	case "-title":
		return "title_desc"
	default:
		return "newest"
	}
}
