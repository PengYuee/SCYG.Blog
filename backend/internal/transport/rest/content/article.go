package content

import (
	"context"
	"fmt"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// ListArticles implements the generated published article list operation.
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

// CreateArticle implements the generated article creation operation.
func (handler *Handler) CreateArticle(ctx context.Context, request generated.CreateArticleRequestObject) (generated.CreateArticleResponseObject, error) {
	body := request.Body
	if body.Status != generated.Draft {
		return nil, invalidETag(fmt.Errorf("new articles must be drafts"))
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

// GetArticle implements the generated published article detail operation.
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

// PatchArticle implements partial article revision and lifecycle transitions.
func (handler *Handler) PatchArticle(ctx context.Context, request generated.PatchArticleRequestObject) (generated.PatchArticleResponseObject, error) {
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

// DeleteArticle implements optimistic article deletion.
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
