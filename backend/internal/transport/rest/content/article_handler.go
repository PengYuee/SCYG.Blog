package content

import (
	"context"
	"fmt"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// ListArticles 实现生成的公开文章列表操作。
func (handler *Handler) ListArticles(ctx context.Context, request generated.ListArticlesRequestObject) (generated.ListArticlesResponseObject, error) {
	page, size := pageValues(request.Params.Page, request.Params.PageSize)
	query := module.ListArticles{Page: page, PageSize: size, Sort: articleSort(request.Params.Sort)}
	if request.Params.ArticleTypeID != nil {
		query.ArticleTypeID = *request.Params.ArticleTypeID
	}
	if request.Params.TagID != nil {
		query.TagID = *request.Params.TagID
	}
	if request.Params.Q != nil {
		query.Query = *request.Params.Q
	}
	result, err := handler.queries.ListArticles(ctx, query)
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

// CreateArticle 实现生成的文章创建操作。
func (handler *Handler) CreateArticle(ctx context.Context, request generated.CreateArticleRequestObject) (generated.CreateArticleResponseObject, error) {
	body := request.Body
	if body.Status != generated.Draft {
		return nil, invalidETag(fmt.Errorf("新建文章必须为草稿状态"))
	}
	tags := make([]int64, len(body.TagIds))
	copy(tags, body.TagIds)
	result, err := handler.commands.CreateArticle(ctx, module.CreateArticle{ArticleTypeID: body.ArticleTypeID, Title: body.Title, Slug: body.Slug, Digest: body.Digest, Content: body.Content, TagIDs: tags})
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
	return generated.CreateArticle201JSONResponse{Body: dto, Headers: generated.CreateArticle201ResponseHeaders{ETag: etag, Location: fmt.Sprintf("/api/v1/articles/%d", result.ID)}}, nil
}

// GetArticle 实现生成的公开文章详情操作。
func (handler *Handler) GetArticle(ctx context.Context, request generated.GetArticleRequestObject) (generated.GetArticleResponseObject, error) {
	result, err := handler.queries.GetArticle(ctx, module.GetArticle{ID: request.ArticleID})
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

// PatchArticle 实现文章局部修订与生命周期迁移；If-Match 必须是强 ETag。
func (handler *Handler) PatchArticle(ctx context.Context, request generated.PatchArticleRequestObject) (generated.PatchArticleResponseObject, error) {
	// 先解析强 ETag，避免未携带有效并发版本的更新进入应用层。
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, &module.ApplicationError{Code: module.CodeValidation, Kind: module.KindValidation, Cause: err}
	}
	body := request.Body
	command := module.PatchArticle{ID: request.ArticleID, Version: version, ArticleTypeID: body.ArticleTypeID, Title: body.Title, Slug: body.Slug, Digest: body.Digest, Content: body.Content}
	if body.TagIds != nil {
		tags := make([]int64, len(*body.TagIds))
		copy(tags, *body.TagIds)
		command.TagIDs = &tags
	}
	if body.Status != nil {
		status := statusName(*body.Status)
		command.Status = &status
	}
	result, err := handler.commands.PatchArticle(ctx, command)
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
	return generated.PatchArticle200JSONResponse{Body: dto, Headers: generated.PatchArticle200ResponseHeaders{ETag: etag}}, nil
}

// DeleteArticle 实现基于乐观锁版本的文章删除。
func (handler *Handler) DeleteArticle(ctx context.Context, request generated.DeleteArticleRequestObject) (generated.DeleteArticleResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, &module.ApplicationError{Code: module.CodeValidation, Kind: module.KindValidation, Cause: err}
	}
	if err = handler.commands.DeleteArticle(ctx, module.DeleteArticle{ID: request.ArticleID, Version: version}); err != nil {
		return nil, err
	}
	return generated.DeleteArticle204Response{}, nil
}
