package content

import (
	"context"
	"fmt"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// ListTags implements the generated tag list operation.
func (handler *Handler) ListTags(ctx context.Context, request generated.ListTagsRequestObject) (generated.ListTagsResponseObject, error) {
	page, size := pageValues(request.Params.Page, request.Params.PageSize)
	query := module.ListTags{Page: page, PageSize: size, Sort: tagSort(request.Params.Sort)}
	if request.Params.Q != nil {
		query.Name = *request.Params.Q
	}
	result, err := handler.queries.ListTags(ctx, query)
	if err != nil {
		return nil, err
	}
	items := make([]generated.Tag, len(result.Items))
	for index, item := range result.Items {
		mapped, mapErr := tagDTO(item)
		if mapErr != nil {
			return nil, mapErr
		}
		items[index] = mapped
	}
	metadata, err := pageInfo(result.Number, result.Size, result.TotalItems, result.TotalPages, len(items))
	if err != nil {
		return nil, err
	}
	return generated.ListTags200JSONResponse{Items: items, Page: metadata}, nil
}

// CreateTag implements the generated tag creation operation.
func (handler *Handler) CreateTag(ctx context.Context, request generated.CreateTagRequestObject) (generated.CreateTagResponseObject, error) {
	result, err := handler.commands.CreateTag(ctx, module.CreateTag{Name: request.Body.Name})
	if err != nil {
		return nil, err
	}
	dto, err := tagDTO(result)
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.CreateTag201JSONResponse{Body: dto, Headers: generated.CreateTag201ResponseHeaders{ETag: etag, Location: fmt.Sprintf("/api/v1/tags/%d", result.ID)}}, nil
}

// GetTag implements the generated tag detail operation.
func (handler *Handler) GetTag(ctx context.Context, request generated.GetTagRequestObject) (generated.GetTagResponseObject, error) {
	result, err := handler.queries.GetTag(ctx, module.GetTag{ID: request.TagID})
	if err != nil {
		return nil, err
	}
	dto, err := tagDTO(result)
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.GetTag200JSONResponse{Body: dto, Headers: generated.GetTag200ResponseHeaders{ETag: etag}}, nil
}

// PatchTag implements optimistic tag rename.
func (handler *Handler) PatchTag(ctx context.Context, request generated.PatchTagRequestObject) (generated.PatchTagResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, invalidETag(err)
	}
	if request.Body.Name == nil {
		return nil, invalidETag(fmt.Errorf("name is required"))
	}
	result, err := handler.commands.RenameTag(ctx, module.RenameTag{ID: request.TagID, Version: version, Name: *request.Body.Name})
	if err != nil {
		return nil, err
	}
	dto, err := tagDTO(result)
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.PatchTag200JSONResponse{Body: dto, Headers: generated.PatchTag200ResponseHeaders{ETag: etag}}, nil
}

// DeleteTag implements optimistic tag deletion.
func (handler *Handler) DeleteTag(ctx context.Context, request generated.DeleteTagRequestObject) (generated.DeleteTagResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, invalidETag(err)
	}
	if err = handler.commands.DeleteTag(ctx, module.DeleteTag{ID: request.TagID, Version: version}); err != nil {
		return nil, err
	}
	return generated.DeleteTag204Response{}, nil
}

func tagSort(value *generated.ListTagsParamsSort) string {
	if value == nil {
		return "title"
	}
	return string(*value)
}
