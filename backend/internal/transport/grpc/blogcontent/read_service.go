package blogcontent

import (
	"context"
	"math"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	agentv1 "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/agent/v1"
	blogv1 "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/blog/v1"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
)

func managementSort(value string) string {
	if value == "" {
		return "-createdAt"
	}
	return value
}

func pageMetadata(number, size, pages int) ([3]uint32, error) {
	var result [3]uint32
	for index, value := range [3]int{number, size, pages} {
		wide := int64(value)
		if wide < 0 || wide > math.MaxUint32 {
			return result, status.Error(codes.Internal, "pagination metadata exceeds protocol bounds")
		}
		result[index] = uint32(wide)
	}
	return result, nil
}

// SearchArticles reads only the explicitly requested content projection.
func (server *Server) SearchArticles(ctx context.Context, request *blogv1.SearchArticlesRequest) (*blogv1.SearchArticlesResponse, error) {
	ctx, err := server.readContext(ctx, request.GetUserId(), request.GetContentScope())
	if err != nil {
		return nil, err
	}
	state, err := filterStatus(request.GetStatus())
	if err != nil {
		return nil, err
	}
	if request.GetContentScope() == agentv1.ContentScope_CONTENT_SCOPE_PUBLIC && state != "" && state != "published" {
		return nil, status.Error(codes.InvalidArgument, "public content requires published status")
	}
	page, size := paging(request.GetPage(), request.GetPageSize())
	query := article.List{Page: page, PageSize: size, Query: request.GetQuery(), Status: state, ArticleTypeID: request.GetArticleTypeId(), TagID: request.GetTagId(), Sort: managementSort(request.GetSort())}
	var result article.Page
	if request.GetContentScope() == agentv1.ContentScope_CONTENT_SCOPE_PUBLIC {
		result, err = server.dependencies.Articles.List(ctx, query)
	} else {
		result, err = server.dependencies.Articles.ListManage(ctx, query)
	}
	if err != nil {
		return nil, rpcError(err)
	}
	items, err := server.articles(ctx, result.Items)
	if err != nil {
		return nil, err
	}
	metadata, err := pageMetadata(result.Number, result.Size, result.TotalPages)
	if err != nil {
		return nil, err
	}
	return &blogv1.SearchArticlesResponse{Articles: items, Page: metadata[0], PageSize: metadata[1], TotalItems: result.TotalItems, TotalPages: metadata[2]}, nil
}

// GetArticle reads only the explicitly requested content projection.
func (server *Server) GetArticle(ctx context.Context, request *blogv1.GetArticleRequest) (*blogv1.GetArticleResponse, error) {
	ctx, err := server.readContext(ctx, request.GetUserId(), request.GetContentScope())
	if err != nil {
		return nil, err
	}
	var result article.Result
	if request.GetContentScope() == agentv1.ContentScope_CONTENT_SCOPE_PUBLIC {
		result, err = server.dependencies.Articles.Get(ctx, article.Get{ID: request.GetArticleId()})
	} else {
		result, err = server.dependencies.Articles.GetManage(ctx, article.Get{ID: request.GetArticleId()})
	}
	item, err := server.oneArticle(ctx, result, err)
	if err != nil {
		return nil, err
	}
	return &blogv1.GetArticleResponse{Article: item}, nil
}

func (server *Server) readContext(ctx context.Context, subject string, scope agentv1.ContentScope) (context.Context, error) {
	switch scope {
	case agentv1.ContentScope_CONTENT_SCOPE_AUTHOR:
		return server.businessContext(ctx, subject)
	case agentv1.ContentScope_CONTENT_SCOPE_PUBLIC:
		return publicContext(ctx, subject)
	case agentv1.ContentScope_CONTENT_SCOPE_UNSPECIFIED:
		fallthrough
	default:
		return nil, status.Error(codes.InvalidArgument, "explicit valid content_scope required")
	}
}

// ListTags authorizes taxonomy management access before querying.
func (server *Server) ListTags(ctx context.Context, request *blogv1.ListTagsRequest) (*blogv1.ListTagsResponse, error) {
	ctx, err := server.businessContext(ctx, request.GetUserId())
	if err != nil {
		return nil, err
	}
	page, size := paging(request.GetPage(), request.GetPageSize())
	result, err := server.dependencies.Taxonomy.ListManageTags(ctx, taxonomy.ListTags{Page: page, PageSize: size, Name: request.GetQuery(), Sort: managementSort(request.GetSort())})
	if err != nil {
		return nil, rpcError(err)
	}
	items := make([]*blogv1.Tag, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, &blogv1.Tag{Id: item.ID, Name: item.Name})
	}
	metadata, err := pageMetadata(result.Number, result.Size, result.TotalPages)
	if err != nil {
		return nil, err
	}
	return &blogv1.ListTagsResponse{Tags: items, Page: metadata[0], PageSize: metadata[1], TotalItems: result.TotalItems, TotalPages: metadata[2]}, nil
}

// ListArticleTypes authorizes management access before a paginated query.
func (server *Server) ListArticleTypes(ctx context.Context, request *blogv1.ListArticleTypesRequest) (*blogv1.ListArticleTypesResponse, error) {
	ctx, err := server.businessContext(ctx, request.GetUserId())
	if err != nil {
		return nil, err
	}
	page, size := paging(request.GetPage(), request.GetPageSize())
	result, err := server.dependencies.Taxonomy.ListManageArticleTypes(ctx, taxonomy.ListArticleTypes{Page: page, PageSize: size, Name: request.GetQuery(), Sort: managementSort(request.GetSort())})
	if err != nil {
		return nil, rpcError(err)
	}
	items := make([]*blogv1.ArticleType, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, &blogv1.ArticleType{Id: item.ID, Name: item.Name})
	}
	metadata, err := pageMetadata(result.Number, result.Size, result.TotalPages)
	if err != nil {
		return nil, err
	}
	return &blogv1.ListArticleTypesResponse{ArticleTypes: items, Page: metadata[0], PageSize: metadata[1], TotalItems: result.TotalItems, TotalPages: metadata[2]}, nil
}
