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
		items[index] = articleDTO(item)
	}
	return generated.ListArticles200JSONResponse{Items: items, Page: generated.PageInfo{Number: int32(result.Number), Size: int32(result.Size), TotalItems: result.TotalItems, TotalPages: int64(result.TotalPages)}}, nil
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
	return generated.CreateArticle201JSONResponse{Body: articleDTO(result), Headers: generated.CreateArticle201ResponseHeaders{ETag: entityTag(result.Version), Location: fmt.Sprintf("/api/v1/articles/%d", result.ID)}}, nil
}

// GetArticle implements the generated published article detail operation.
func (handler *Handler) GetArticle(ctx context.Context, request generated.GetArticleRequestObject) (generated.GetArticleResponseObject, error) {
	result, err := handler.queries.GetArticle(ctx, module.GetArticle{ID: request.ArticleID})
	if err != nil {
		return nil, err
	}
	return generated.GetArticle200JSONResponse{Body: articleDTO(result), Headers: generated.GetArticle200ResponseHeaders{ETag: entityTag(result.Version)}}, nil
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
	return generated.PatchArticle200JSONResponse{Body: articleDTO(result), Headers: generated.PatchArticle200ResponseHeaders{ETag: entityTag(result.Version)}}, nil
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
