package blogcontent

import (
	"context"
	"strings"

	blogv1 "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/blog/v1"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
)

func (server *Server) writeContext(ctx context.Context, userID, key string) (context.Context, error) {
	ctx, err := server.businessContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := operationID(key); err != nil {
		return nil, err
	}
	return ctx, nil
}

// CreateArticle arbitrates a global key before executing Blog creation rules.
func (server *Server) CreateArticle(ctx context.Context, request *blogv1.CreateArticleRequest) (*blogv1.CreateArticleResponse, error) {
	ctx, err := server.writeContext(ctx, request.GetUserId(), request.GetOperationId())
	if err != nil {
		return nil, err
	}
	input := article.Create{Status: article.ArticleCreationStatus(request.GetStatus()), ArticleTypeID: request.GetArticleTypeId(), Title: request.GetTitle(), Slug: request.GetSlug(), Digest: request.GetDigest(), Content: request.GetContent(), TagIDs: request.GetTagIds()}
	result, err := server.dependencies.Operations.Create(ctx, strings.ToLower(request.GetOperationId()), input)
	item, err := server.oneArticle(ctx, result, err)
	if err != nil {
		return nil, err
	}
	return &blogv1.CreateArticleResponse{Article: item}, nil
}

// UpdateArticle preserves absent versus explicitly empty tags and scalar fields.
func (server *Server) UpdateArticle(ctx context.Context, request *blogv1.UpdateArticleRequest) (*blogv1.UpdateArticleResponse, error) {
	ctx, err := server.writeContext(ctx, request.GetUserId(), request.GetOperationId())
	if err != nil {
		return nil, err
	}
	input := article.Patch{ID: request.GetArticleId(), Version: request.GetExpectedVersion(), ArticleTypeID: request.ArticleTypeId, Title: request.Title, Slug: request.Slug, Digest: request.Digest, Content: request.Content}
	if request.TagIds != nil {
		ids := request.TagIds.Values
		input.TagIDs = &ids
	}
	result, err := server.dependencies.Operations.Patch(ctx, strings.ToLower(request.GetOperationId()), input)
	item, err := server.oneArticle(ctx, result, err)
	if err != nil {
		return nil, err
	}
	return &blogv1.UpdateArticleResponse{Article: item}, nil
}

// PublishArticle replays current state before validating a new expected version.
func (server *Server) PublishArticle(ctx context.Context, request *blogv1.PublishArticleRequest) (*blogv1.PublishArticleResponse, error) {
	ctx, err := server.writeContext(ctx, request.GetUserId(), request.GetOperationId())
	if err != nil {
		return nil, err
	}
	result, err := server.dependencies.Operations.Publish(ctx, strings.ToLower(request.GetOperationId()), article.Publish{ID: request.GetArticleId(), Version: request.GetExpectedVersion()})
	item, err := server.oneArticle(ctx, result, err)
	if err != nil {
		return nil, err
	}
	return &blogv1.PublishArticleResponse{Article: item}, nil
}

// ArchiveArticle applies the existing archival lifecycle rules after arbitration.
func (server *Server) ArchiveArticle(ctx context.Context, request *blogv1.ArchiveArticleRequest) (*blogv1.ArchiveArticleResponse, error) {
	ctx, err := server.writeContext(ctx, request.GetUserId(), request.GetOperationId())
	if err != nil {
		return nil, err
	}
	result, err := server.dependencies.Operations.Archive(ctx, strings.ToLower(request.GetOperationId()), article.Archive{ID: request.GetArticleId(), Version: request.GetExpectedVersion()})
	item, err := server.oneArticle(ctx, result, err)
	if err != nil {
		return nil, err
	}
	return &blogv1.ArchiveArticleResponse{Article: item}, nil
}
