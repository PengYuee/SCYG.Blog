package content

import (
	"context"
	"fmt"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// ListArticleTypes 实现生成的文章分类列表操作。
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
	return generated.ListArticleTypes200JSONResponse{Items: items, Page: metadata}, nil
}

// CreateArticleType 实现生成的文章分类创建操作。
func (handler *Handler) CreateArticleType(ctx context.Context, request generated.CreateArticleTypeRequestObject) (generated.CreateArticleTypeResponseObject, error) {
	result, err := handler.commands.CreateArticleType(ctx, module.CreateArticleType{Name: request.Body.Name, Image: request.Body.Image, Meun: request.Body.Meun})
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
	return generated.CreateArticleType201JSONResponse{Body: dto, Headers: generated.CreateArticleType201ResponseHeaders{ETag: etag, Location: fmt.Sprintf("/api/v1/article-types/%d", result.ID)}}, nil
}

// GetArticleType 实现生成的文章分类详情操作。
func (handler *Handler) GetArticleType(ctx context.Context, request generated.GetArticleTypeRequestObject) (generated.GetArticleTypeResponseObject, error) {
	result, err := handler.queries.GetArticleType(ctx, module.GetArticleType{ID: request.ArticleTypeID})
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
	return generated.GetArticleType200JSONResponse{Body: dto, Headers: generated.GetArticleType200ResponseHeaders{ETag: etag}}, nil
}

// PatchArticleType 实现基于强 ETag 的文章分类局部更新。
func (handler *Handler) PatchArticleType(ctx context.Context, request generated.PatchArticleTypeRequestObject) (generated.PatchArticleTypeResponseObject, error) {
	version, err := parseEntityTag(request.Params.IfMatch)
	if err != nil {
		return nil, invalidETag(err)
	}
	image, _ := ctx.Value(articleTypeImageKey).(imagePatch)
	result, err := handler.commands.PatchArticleType(ctx, module.PatchArticleType{ID: request.ArticleTypeID, Version: version, Name: request.Body.Name, Image: module.OptionalImage{Provided: image.provided, Value: image.value}, Meun: request.Body.Meun})
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
	return generated.PatchArticleType200JSONResponse{Body: dto, Headers: generated.PatchArticleType200ResponseHeaders{ETag: etag}}, nil
}

// DeleteArticleType 实现基于乐观锁版本的文章分类删除。
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
