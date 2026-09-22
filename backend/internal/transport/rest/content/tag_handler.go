package content

import (
	"context"
	"fmt"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
)

// ListPublicTags implements the public tag list operation.
func (handler *Handler) ListPublicTags(ctx context.Context, request generated.ListPublicTagsRequestObject) (generated.ListPublicTagsResponseObject, error) {
	query := taxonomy.ListPublicTags{Page: int(request.Params.Page), PageSize: int(request.Params.PageSize)}
	if request.Params.Q != nil {
		query.Name = *request.Params.Q
	}
	result, err := handler.taxonomy.ListPublicTags(requestContext(ctx), query)
	if err != nil {
		return nil, err
	}
	items := make([]generated.PublicTag, len(result.Items))
	for index, item := range result.Items {
		items[index] = publicTagDTO(item)
	}
	metadata, err := pageInfo(result.Number, result.Size, result.TotalItems, result.TotalPages, len(items))
	if err != nil {
		return nil, err
	}
	return generated.ListPublicTags200JSONResponse{Items: items, Page: metadata}, nil
}

// GetPublicTag implements the public tag detail operation.
func (handler *Handler) GetPublicTag(ctx context.Context, request generated.GetPublicTagRequestObject) (generated.GetPublicTagResponseObject, error) {
	result, err := handler.taxonomy.GetPublicTag(requestContext(ctx), taxonomy.GetPublicTag{ID: request.TagID})
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.GetPublicTag200JSONResponse{Body: publicTagDTO(result), Headers: generated.GetPublicTag200ResponseHeaders{ETag: etag}}, nil
}

// ListManageTags implements the protected tag list operation.
func (handler *Handler) ListManageTags(ctx context.Context, request generated.ListManageTagsRequestObject) (generated.ListManageTagsResponseObject, error) {
	page, size := pageValues(request.Params.Page, request.Params.PageSize)
	query := taxonomy.ListTags{Page: page, PageSize: size, Sort: sortValue(request.Params.Sort)}
	if request.Params.Q != nil {
		query.Name = *request.Params.Q
	}
	result, err := handler.taxonomy.ListTags(requestContext(ctx), query)
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
	return generated.ListManageTags200JSONResponse{Items: items, Page: metadata}, nil
}

// CreateManageTag implements the protected tag creation operation.
func (handler *Handler) CreateManageTag(ctx context.Context, request generated.CreateManageTagRequestObject) (generated.CreateManageTagResponseObject, error) {
	result, err := handler.taxonomy.CreateTag(requestContext(ctx), taxonomy.CreateTag{Name: request.Body.Name})
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
	return generated.CreateManageTag201JSONResponse{Body: dto, Headers: generated.CreateManageTag201ResponseHeaders{ETag: etag, Location: fmt.Sprintf("/api/v1/manage/tags/%d", result.ID)}}, nil
}

// GetManageTag implements the protected tag detail operation.
func (handler *Handler) GetManageTag(ctx context.Context, request generated.GetManageTagRequestObject) (generated.GetManageTagResponseObject, error) {
	result, err := handler.taxonomy.GetTag(requestContext(ctx), taxonomy.GetTag{ID: request.TagID})
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
	return generated.GetManageTag200JSONResponse{Body: dto, Headers: generated.GetManageTag200ResponseHeaders{ETag: etag}}, nil
}

// PatchManageTag implements the protected tag rename operation.
func (handler *Handler) PatchManageTag(ctx context.Context, request generated.PatchManageTagRequestObject) (generated.PatchManageTagResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, invalidETag(err)
	}
	if request.Body.Name == nil {
		return nil, invalidETag(fmt.Errorf("必须提供标签名称"))
	}
	result, err := handler.taxonomy.RenameTag(requestContext(ctx), taxonomy.RenameTag{ID: request.TagID, Version: version, Name: *request.Body.Name})
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
	return generated.PatchManageTag200JSONResponse{Body: dto, Headers: generated.PatchManageTag200ResponseHeaders{ETag: etag}}, nil
}

// DeleteManageTag implements the protected tag deletion operation.
func (handler *Handler) DeleteManageTag(ctx context.Context, request generated.DeleteManageTagRequestObject) (generated.DeleteManageTagResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, invalidETag(err)
	}
	if err = handler.taxonomy.DeleteTag(requestContext(ctx), taxonomy.DeleteTag{ID: request.TagID, Version: version}); err != nil {
		return nil, err
	}
	return generated.DeleteManageTag204Response{}, nil
}

func publicTagDTO(value taxonomy.PublicTagResult) generated.PublicTag {
	return generated.PublicTag{ID: value.ID, Name: value.Name, ArticleCount: value.ArticleCount}
}
