package content

import (
	"context"
	"fmt"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
)

// ListArticles 实现生成的公开文章列表操作。
func (handler *Handler) ListArticles(ctx context.Context, request generated.ListArticlesRequestObject) (generated.ListArticlesResponseObject, error) {
	page, size := pageValues(request.Params.Page, request.Params.PageSize)
	query := article.List{Page: page, PageSize: size, Sort: sortValue(request.Params.Sort)}
	if request.Params.ArticleTypeID != nil {
		query.ArticleTypeID = *request.Params.ArticleTypeID
	}
	if request.Params.TagID != nil {
		query.TagID = *request.Params.TagID
	}
	if request.Params.Q != nil {
		query.Query = *request.Params.Q
	}
	result, err := handler.articleQueries.List(requestContext(ctx), query)
	if err != nil {
		return nil, err
	}
	items := make([]generated.Article, len(result.Items))
	for index, item := range result.Items {
		mapped, mapErr := articleDTO(item)
		if mapErr != nil {
			return nil, mapErr
		}
		items[index] = mapped
	}
	metadata, err := pageInfo(result.Number, result.Size, result.TotalItems, result.TotalPages, len(items))
	if err != nil {
		return nil, err
	}
	return generated.ListArticles200JSONResponse{Items: items, Page: metadata}, nil
}

// CreateManageArticle 实现管理端文章创建操作。
func (handler *Handler) CreateManageArticle(ctx context.Context, request generated.CreateManageArticleRequestObject) (generated.CreateManageArticleResponseObject, error) {
	body := request.Body
	var status article.ArticleCreationStatus
	// 生成层创建枚举必须穷尽映射，防止绕过契约中间件的伪造值进入应用层。
	switch body.Status {
	case generated.ArticleCreateStatusDraft:
		status = article.ArticleCreationStatusDraft
	case generated.ArticleCreateStatusPublished:
		status = article.ArticleCreationStatusPublished
	default:
		return nil, newRESTError(codeValidation, fmt.Errorf("文章创建状态 %d 不合法", body.Status))
	}
	tags := make([]int64, len(body.TagIds))
	copy(tags, body.TagIds)
	result, err := handler.articleCommands.Create(requestContext(ctx), article.Create{Status: status, ArticleTypeID: body.ArticleTypeID, Title: body.Title, Slug: body.Slug, Digest: body.Digest, Content: body.Content, TagIDs: tags})
	if err != nil {
		return nil, err
	}
	dto, err := articleDTO(result)
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.CreateManageArticle201JSONResponse{Body: dto, Headers: generated.CreateManageArticle201ResponseHeaders{ETag: etag, Location: fmt.Sprintf("/api/v1/manage/articles/%d", result.ID)}}, nil
}

// GetArticle 实现生成的公开文章详情操作。
func (handler *Handler) GetArticle(ctx context.Context, request generated.GetArticleRequestObject) (generated.GetArticleResponseObject, error) {
	result, err := handler.articleQueries.Get(requestContext(ctx), article.Get{ID: request.ArticleID})
	if err != nil {
		return nil, err
	}
	dto, err := articleDTO(result)
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.GetArticle200JSONResponse{Body: dto, Headers: generated.GetArticle200ResponseHeaders{ETag: etag}}, nil
}

// PatchManageArticle 实现管理端文章局部修订；If-Match 必须是强 ETag。
func (handler *Handler) PatchManageArticle(ctx context.Context, request generated.PatchManageArticleRequestObject) (generated.PatchManageArticleResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, newRESTError(codeValidation, err)
	}
	body := request.Body
	command := article.Patch{ID: request.ArticleID, Version: version, ArticleTypeID: body.ArticleTypeID, Title: body.Title, Slug: body.Slug, Digest: body.Digest, Content: body.Content}
	if body.TagIds != nil {
		tags := make([]int64, len(*body.TagIds))
		copy(tags, *body.TagIds)
		command.TagIDs = &tags
	}
	result, err := handler.articleCommands.Patch(requestContext(ctx), command)
	if err != nil {
		return nil, err
	}
	dto, err := articleDTO(result)
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.PatchManageArticle200JSONResponse{Body: dto, Headers: generated.PatchManageArticle200ResponseHeaders{ETag: etag}}, nil
}

// DeleteManageArticle 实现管理端文章删除。
func (handler *Handler) DeleteManageArticle(ctx context.Context, request generated.DeleteManageArticleRequestObject) (generated.DeleteManageArticleResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, newRESTError(codeValidation, err)
	}
	if err = handler.articleDeleter.Delete(requestContext(ctx), article.Delete{ID: request.ArticleID, Version: version}); err != nil {
		return nil, err
	}
	return generated.DeleteManageArticle204Response{}, nil
}

// ListManageArticles implements the protected article management list.
func (handler *Handler) ListManageArticles(ctx context.Context, request generated.ListManageArticlesRequestObject) (generated.ListManageArticlesResponseObject, error) {
	page, size := pageValues(request.Params.Page, request.Params.PageSize)
	query := article.List{Page: page, PageSize: size, Sort: sortValue(request.Params.Sort)}
	if request.Params.ArticleTypeID != nil {
		query.ArticleTypeID = *request.Params.ArticleTypeID
	}
	if request.Params.TagID != nil {
		query.TagID = *request.Params.TagID
	}
	if request.Params.Q != nil {
		query.Query = *request.Params.Q
	}
	result, err := handler.articleQueries.ListManage(requestContext(ctx), query)
	if err != nil {
		return nil, err
	}
	return manageArticleListResponse(result)
}

// GetManageArticle implements the protected article management detail.
func (handler *Handler) GetManageArticle(ctx context.Context, request generated.GetManageArticleRequestObject) (generated.GetManageArticleResponseObject, error) {
	result, err := handler.articleQueries.GetManage(requestContext(ctx), article.Get{ID: request.ArticleID})
	if err != nil {
		return nil, err
	}
	dto, err := articleDTO(result)
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.GetManageArticle200JSONResponse{Body: dto, Headers: generated.GetManageArticle200ResponseHeaders{ETag: etag}}, nil
}

// PublishManageArticle publishes a draft using the supplied strong ETag.
func (handler *Handler) PublishManageArticle(ctx context.Context, request generated.PublishManageArticleRequestObject) (generated.PublishManageArticleResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, newRESTError(codeValidation, err)
	}
	result, err := handler.articleCommands.Publish(requestContext(ctx), article.Publish{ID: request.ArticleID, Version: version})
	if err != nil {
		return nil, err
	}
	dto, err := articleDTO(result)
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.PublishManageArticle200JSONResponse{Body: dto, Headers: generated.PublishManageArticle200ResponseHeaders{ETag: etag}}, nil
}

// ArchiveManageArticle archives a published article using the supplied strong ETag.
func (handler *Handler) ArchiveManageArticle(ctx context.Context, request generated.ArchiveManageArticleRequestObject) (generated.ArchiveManageArticleResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, newRESTError(codeValidation, err)
	}
	result, err := handler.articleCommands.Archive(requestContext(ctx), article.Archive{ID: request.ArticleID, Version: version})
	if err != nil {
		return nil, err
	}
	dto, err := articleDTO(result)
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.ArchiveManageArticle200JSONResponse{Body: dto, Headers: generated.ArchiveManageArticle200ResponseHeaders{ETag: etag}}, nil
}

func manageArticleListResponse(result article.Page) (generated.ListManageArticlesResponseObject, error) {
	items := make([]generated.Article, len(result.Items))
	for index, item := range result.Items {
		mapped, err := articleDTO(item)
		if err != nil {
			return nil, err
		}
		items[index] = mapped
	}
	metadata, err := pageInfo(result.Number, result.Size, result.TotalItems, result.TotalPages, len(items))
	if err != nil {
		return nil, err
	}
	return generated.ListManageArticles200JSONResponse{Items: items, Page: metadata}, nil
}
