package content

import (
	"time"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
)

func articleDTO(item article.Result) (generated.Article, error) {
	summary := generated.PublicArticleTypeSummary{ID: item.ArticleTypeID, Name: item.ArticleTypeName, Image: item.ArticleTypeImage}
	// 响应离开应用边界前必须重新校验，防止持久化异常数据突破 OpenAPI 契约。
	version, err := generatedVersion(item.Version)
	if err != nil || articleResponseTextInvalid(item) || item.ID <= 0 || item.ArticleTypeID <= 0 || summary.Name == "" || len([]rune(summary.Name)) > 60 || invalidTimes(item.CreatedAt, item.ModifiedAt) || item.Support < 0 || item.Comment < 0 || item.Visited < 0 {
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
	return generated.Article{ID: item.ID, ArticleTypeID: item.ArticleTypeID, ArticleType: summary, Title: item.Title, Slug: item.Slug, Digest: item.Digest, Content: item.Content, Status: status, TagIds: tags, Support: item.Support, Comment: item.Comment, Visited: item.Visited, Version: version, CreatedAt: item.CreatedAt, UpdatedAt: updated}, nil
}

func articleListDTO(values []article.Result) ([]generated.Article, error) {
	items := make([]generated.Article, len(values))
	for index, item := range values {
		mapped, err := articleDTO(item)
		if err != nil {
			return nil, err
		}
		items[index] = mapped
	}
	return items, nil
}

func articleResponseTextInvalid(item article.Result) bool {
	title, titleErr := article.NewTitle(item.Title)
	slug, slugErr := article.NewSlug(item.Slug)
	digest, digestErr := article.NewDigest(item.Digest)
	body, bodyErr := article.NewContent(item.Content)
	return titleErr != nil || slugErr != nil || digestErr != nil || bodyErr != nil || title.String() != item.Title || slug.String() != item.Slug || digest.String() != item.Digest || body.String() != item.Content
}
