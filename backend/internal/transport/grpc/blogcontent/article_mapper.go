package blogcontent

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	blogv1 "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/blog/v1"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
)

func (server *Server) articles(ctx context.Context, items []article.Result) ([]*blogv1.Article, error) {
	ids := make([]int64, 0)
	seen := make(map[int64]struct{})
	for _, item := range items {
		for _, id := range item.TagIDs {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	tags, err := server.dependencies.Taxonomy.TagsByIDs(ctx, ids)
	if err != nil {
		return nil, rpcError(err)
	}
	names := make(map[int64]string, len(tags))
	for _, tag := range tags {
		names[tag.ID] = tag.Name
	}
	out := make([]*blogv1.Article, 0, len(items))
	for _, item := range items {
		state, err := wireStatus(item.Status)
		if err != nil {
			return nil, err
		}
		value := &blogv1.Article{Id: item.ID, ArticleTypeId: item.ArticleTypeID, Title: item.Title, Slug: item.Slug, Digest: item.Digest, Content: item.Content, Status: state, Version: item.Version, CreatedAt: timestamppb.New(item.CreatedAt), UpdatedAt: timestamppb.New(item.ModifiedAt), Support: item.Support, Comment: item.Comment, Visited: item.Visited}
		for _, id := range item.TagIDs {
			if name, ok := names[id]; ok {
				value.Tags = append(value.Tags, &blogv1.Tag{Id: id, Name: name})
			}
		}
		// The domain does not persist a publication timestamp. Leave it absent rather
		// than manufacture a timestamp from a later update.
		out = append(out, value)
	}
	return out, nil
}

func (server *Server) oneArticle(ctx context.Context, item article.Result, err error) (*blogv1.Article, error) {
	if err != nil {
		return nil, rpcError(err)
	}
	items, err := server.articles(ctx, []article.Result{item})
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

func wireStatus(value string) (blogv1.ArticleStatus, error) {
	switch value {
	case "draft":
		return blogv1.ArticleStatus_ARTICLE_STATUS_DRAFT, nil
	case "published":
		return blogv1.ArticleStatus_ARTICLE_STATUS_PUBLISHED, nil
	case "archived":
		return blogv1.ArticleStatus_ARTICLE_STATUS_ARCHIVED, nil
	default:
		return 0, status.Error(codes.Internal, "invalid article state")
	}
}

func filterStatus(value blogv1.ArticleStatus) (string, error) {
	switch value {
	case blogv1.ArticleStatus_ARTICLE_STATUS_UNSPECIFIED:
		return "", nil
	case blogv1.ArticleStatus_ARTICLE_STATUS_DRAFT:
		return "draft", nil
	case blogv1.ArticleStatus_ARTICLE_STATUS_PUBLISHED:
		return "published", nil
	case blogv1.ArticleStatus_ARTICLE_STATUS_ARCHIVED:
		return "archived", nil
	default:
		return "", status.Error(codes.InvalidArgument, "invalid status")
	}
}

func paging(page, size uint32) (int, int) {
	if page == 0 {
		page = 1
	}
	if size == 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return int(page), int(size)
}
