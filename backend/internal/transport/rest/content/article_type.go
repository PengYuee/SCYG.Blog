package content

import (
	"context"
	"fmt"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// ListArticleTypes implements the generated article-type list operation.
func (handler *Handler) ListArticleTypes(ctx context.Context, request generated.ListArticleTypesRequestObject) (generated.ListArticleTypesResponseObject, error) {
	page, size := pageValues(request.Params.Page, request.Params.PageSize)
	query := module.ListArticleTypes{Page: page, PageSize: size, Sort: taxonomySort(request.Params.Sort)}
	if request.Params.Q != nil {
		query.Name = *request.Params.Q
	}
	result, err := handler.queries.ListArticleTypes(ctx, query)
	if err != nil {
		return nil, err
	}
	items := make([]generated.ArticleType, len(result.Items))
	for index, item := range result.Items {
		items[index] = articleTypeDTO(item)
	}
	return generated.ListArticleTypes200JSONResponse{Items: items, Page: generated.PageInfo{Number: int32(result.Number), Size: int32(result.Size), TotalItems: result.TotalItems, TotalPages: int64(result.TotalPages)}}, nil
}

// CreateArticleType implements the generated article-type creation operation.
func (handler *Handler) CreateArticleType(ctx context.Context, request generated.CreateArticleTypeRequestObject) (generated.CreateArticleTypeResponseObject, error) {
	result, err := handler.commands.CreateArticleType(ctx, module.CreateArticleType{Name: request.Body.Name})
	if err != nil {
		return nil, err
	}
	return generated.CreateArticleType201JSONResponse{Body: articleTypeDTO(result), Headers: generated.CreateArticleType201ResponseHeaders{ETag: entityTag(result.Version), Location: fmt.Sprintf("/api/v1/article-types/%d", result.ID)}}, nil
}

// GetArticleType implements the generated article-type detail operation.
func (handler *Handler) GetArticleType(ctx context.Context, request generated.GetArticleTypeRequestObject) (generated.GetArticleTypeResponseObject, error) {
	result, err := handler.queries.GetArticleType(ctx, module.GetArticleType{ID: request.ArticleTypeID})
	if err != nil {
		return nil, err
	}
	return generated.GetArticleType200JSONResponse{Body: articleTypeDTO(result), Headers: generated.GetArticleType200ResponseHeaders{ETag: entityTag(result.Version)}}, nil
}

// PatchArticleType implements optimistic article-type rename.
func (handler *Handler) PatchArticleType(ctx context.Context, request generated.PatchArticleTypeRequestObject) (generated.PatchArticleTypeResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, invalidETag(err)
	}
	if request.Body.Name == nil {
		return nil, invalidETag(fmt.Errorf("name is required"))
	}
	result, err := handler.commands.RenameArticleType(ctx, module.RenameArticleType{ID: request.ArticleTypeID, Version: version, Name: *request.Body.Name})
	if err != nil {
		return nil, err
	}
	return generated.PatchArticleType200JSONResponse{Body: articleTypeDTO(result), Headers: generated.PatchArticleType200ResponseHeaders{ETag: entityTag(result.Version)}}, nil
}

// DeleteArticleType implements optimistic article-type deletion.
func (handler *Handler) DeleteArticleType(ctx context.Context, request generated.DeleteArticleTypeRequestObject) (generated.DeleteArticleTypeResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, invalidETag(err)
	}
	if err = handler.commands.DeleteArticleType(ctx, module.DeleteArticleType{ID: request.ArticleTypeID, Version: version}); err != nil {
		return nil, err
	}
	return generated.DeleteArticleType204Response{}, nil
}

func taxonomySort(value *generated.ListArticleTypesParamsSort) string {
	if value == nil {
		return "title"
	}
	return string(*value)
}
func invalidETag(err error) error {
	return &module.ApplicationError{Code: module.CodeValidation, Kind: module.KindValidation, Cause: err}
}
