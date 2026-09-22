package content

import (
	"context"
	"fmt"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
)

// ListPublicArticleTypes implements the public article type list operation.
func (handler *Handler) ListPublicArticleTypes(ctx context.Context, request generated.ListPublicArticleTypesRequestObject) (generated.ListPublicArticleTypesResponseObject, error) {
	query := taxonomy.ListPublicArticleTypes{Page: int(request.Params.Page), PageSize: int(request.Params.PageSize)}
	if request.Params.Q != nil {
		query.Name = *request.Params.Q
	}
	result, err := handler.taxonomy.ListPublicArticleTypes(requestContext(ctx), query)
	if err != nil {
		return nil, err
	}
	items := make([]generated.PublicArticleType, len(result.Items))
	for index, item := range result.Items {
		items[index] = publicArticleTypeDTO(item)
	}
	metadata, err := pageInfo(result.Number, result.Size, result.TotalItems, result.TotalPages, len(items))
	if err != nil {
		return nil, err
	}
	return generated.ListPublicArticleTypes200JSONResponse{Items: items, Page: metadata}, nil
}

// GetPublicArticleType implements the public article type detail operation.
func (handler *Handler) GetPublicArticleType(ctx context.Context, request generated.GetPublicArticleTypeRequestObject) (generated.GetPublicArticleTypeResponseObject, error) {
	result, err := handler.taxonomy.GetPublicArticleType(requestContext(ctx), taxonomy.GetPublicArticleType{ID: request.ArticleTypeID})
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.GetPublicArticleType200JSONResponse{Body: publicArticleTypeDTO(result), Headers: generated.GetPublicArticleType200ResponseHeaders{ETag: etag}}, nil
}

// ListManageArticleTypes implements the protected article type list operation.
func (handler *Handler) ListManageArticleTypes(ctx context.Context, request generated.ListManageArticleTypesRequestObject) (generated.ListManageArticleTypesResponseObject, error) {
	page, size := pageValues(request.Params.Page, request.Params.PageSize)
	query := taxonomy.ListArticleTypes{Page: page, PageSize: size, Sort: sortValue(request.Params.Sort)}
	if request.Params.Q != nil {
		query.Name = *request.Params.Q
	}
	result, err := handler.taxonomy.ListArticleTypes(requestContext(ctx), query)
	if err != nil {
		return nil, err
	}
	items := make([]generated.ArticleType, len(result.Items))
	for index, item := range result.Items {
		mapped, mapErr := articleTypeDTO(item)
		if mapErr != nil {
			return nil, mapErr
		}
		items[index] = mapped
	}
	metadata, err := pageInfo(result.Number, result.Size, result.TotalItems, result.TotalPages, len(items))
	if err != nil {
		return nil, err
	}
	return generated.ListManageArticleTypes200JSONResponse{Items: items, Page: metadata}, nil
}

// CreateManageArticleType implements the protected article type creation operation.
func (handler *Handler) CreateManageArticleType(ctx context.Context, request generated.CreateManageArticleTypeRequestObject) (generated.CreateManageArticleTypeResponseObject, error) {
	result, err := handler.taxonomy.CreateArticleType(requestContext(ctx), taxonomy.CreateArticleType{Name: request.Body.Name, Image: request.Body.Image, Meun: request.Body.Menu})
	if err != nil {
		return nil, err
	}
	dto, err := articleTypeDTO(result)
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.CreateManageArticleType201JSONResponse{Body: dto, Headers: generated.CreateManageArticleType201ResponseHeaders{ETag: etag, Location: fmt.Sprintf("/api/v1/manage/article-types/%d", result.ID)}}, nil
}

// GetManageArticleType implements the protected article type detail operation.
func (handler *Handler) GetManageArticleType(ctx context.Context, request generated.GetManageArticleTypeRequestObject) (generated.GetManageArticleTypeResponseObject, error) {
	result, err := handler.taxonomy.GetArticleType(requestContext(ctx), taxonomy.GetArticleType{ID: request.ArticleTypeID})
	if err != nil {
		return nil, err
	}
	dto, err := articleTypeDTO(result)
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.GetManageArticleType200JSONResponse{Body: dto, Headers: generated.GetManageArticleType200ResponseHeaders{ETag: etag}}, nil
}

// PatchManageArticleType implements the protected article type patch operation.
func (handler *Handler) PatchManageArticleType(ctx context.Context, request generated.PatchManageArticleTypeRequestObject) (generated.PatchManageArticleTypeResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, invalidETag(err)
	}
	image, _ := ctx.Value(articleTypeImageKey).(imagePatch)
	result, err := handler.taxonomy.PatchArticleType(requestContext(ctx), taxonomy.PatchArticleType{ID: request.ArticleTypeID, Version: version, Name: request.Body.Name, Image: taxonomy.OptionalImage{Provided: image.provided, Value: image.value}, Meun: request.Body.Menu})
	if err != nil {
		return nil, err
	}
	dto, err := articleTypeDTO(result)
	if err != nil {
		return nil, err
	}
	etag, err := entityTag(result.Version)
	if err != nil {
		return nil, err
	}
	return generated.PatchManageArticleType200JSONResponse{Body: dto, Headers: generated.PatchManageArticleType200ResponseHeaders{ETag: etag}}, nil
}

// DeleteManageArticleType implements the protected article type deletion operation.
func (handler *Handler) DeleteManageArticleType(ctx context.Context, request generated.DeleteManageArticleTypeRequestObject) (generated.DeleteManageArticleTypeResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, invalidETag(err)
	}
	if err = handler.taxonomy.DeleteArticleType(requestContext(ctx), taxonomy.DeleteArticleType{ID: request.ArticleTypeID, Version: version}); err != nil {
		return nil, err
	}
	return generated.DeleteManageArticleType204Response{}, nil
}

func publicArticleTypeDTO(value taxonomy.PublicArticleTypeResult) generated.PublicArticleType {
	return generated.PublicArticleType{ID: value.ID, Name: value.Name, Image: value.Image, ArticleCount: value.ArticleCount}
}
