//go:build integration

package application_test

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	blogv1 "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/blog/v1"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
	"github.com/PengYuee/SCYG.Blog/backend/internal/transport/grpc/blogcontent"
)

func TestBlogContentGRPCManagementLifecycle(t *testing.T) {
	fixture, operations, articles, _, policy := operationFixture(t)
	taxonomies, err := taxonomy.New(fixture.db.GORM(), taxonomy.Dependencies{Clock: fixture.clock, Authorizer: policy})
	if err != nil {
		t.Fatal(err)
	}
	for index, id := range []string{operationUser, "11111111111111111111111111111111"} {
		if err := fixture.db.GORM().Exec(`INSERT INTO users (id, username, password_hash, is_active, created_at) VALUES (?, ?, ?, true, clock_timestamp())`, id, []string{"grpc-writer", "grpc-reader"}[index], "test-account-hash").Error; err != nil {
			t.Fatal(err)
		}
	}
	users, err := user.NewRepository(fixture.db.GORM())
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	if _, err := blogcontent.Register(server, blogcontent.Dependencies{Articles: articles, Taxonomy: taxonomies, Operations: operations, Users: users}); err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1024 * 1024)
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close(); <-serveErrors })
	connection, err := grpc.NewClient("passthrough:///blog-content", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := blogv1.NewBlogContentServiceClient(connection)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	key := "50000000-0000-4000-8000-000000000001"
	created, err := client.CreateArticle(ctx, &blogv1.CreateArticleRequest{UserId: operationUser, OperationId: key, Status: blogv1.ArticleStatus_ARTICLE_STATUS_DRAFT, ArticleTypeId: fixture.typeID, Title: "grpc-article", Slug: "grpc-article", Digest: "digest", Content: "body", TagIds: []int64{fixture.tagID}})
	if err != nil {
		t.Fatal(err)
	}
	first := created.Article
	if len(first.Tags) != 1 || first.Tags[0].Id != fixture.tagID || first.Tags[0].Name != "application-test-tag" {
		t.Fatalf("tag projection=%+v", first.Tags)
	}
	patched, err := client.UpdateArticle(ctx, &blogv1.UpdateArticleRequest{UserId: operationUser, OperationId: "50000000-0000-4000-8000-000000000002", ArticleId: first.Id, ExpectedVersion: first.Version, Title: new("grpc-patched")})
	if err != nil || len(patched.GetArticle().GetTags()) != 1 {
		t.Fatalf("absent tags=%+v err=%v", patched, err)
	}
	cleared, err := client.UpdateArticle(ctx, &blogv1.UpdateArticleRequest{UserId: operationUser, OperationId: "50000000-0000-4000-8000-000000000003", ArticleId: first.Id, ExpectedVersion: patched.Article.Version, TagIds: &blogv1.TagIds{}})
	if err != nil || len(cleared.GetArticle().GetTags()) != 0 {
		t.Fatalf("empty tags=%+v err=%v", cleared, err)
	}
	published, err := client.PublishArticle(ctx, &blogv1.PublishArticleRequest{UserId: operationUser, OperationId: "50000000-0000-4000-8000-000000000004", ArticleId: first.Id, ExpectedVersion: cleared.Article.Version})
	if err != nil || published.GetArticle().GetStatus() != blogv1.ArticleStatus_ARTICLE_STATUS_PUBLISHED {
		t.Fatalf("publish=%+v err=%v", published, err)
	}
	replay, err := client.UpdateArticle(ctx, &blogv1.UpdateArticleRequest{UserId: operationUser, OperationId: key, ArticleId: -1, ExpectedVersion: 0, Title: new("")})
	if err != nil || replay.GetArticle().GetVersion() != published.Article.Version {
		t.Fatalf("advanced replay=%+v err=%v", replay, err)
	}
	allowedReplay, err := client.CreateArticle(ctx, &blogv1.CreateArticleRequest{UserId: "11111111111111111111111111111111", OperationId: key})
	if err != nil || allowedReplay.GetArticle().GetId() != first.Id {
		t.Fatalf("current-reader replay=%+v err=%v", allowedReplay, err)
	}
	archived, err := client.ArchiveArticle(ctx, &blogv1.ArchiveArticleRequest{UserId: operationUser, OperationId: "50000000-0000-4000-8000-000000000005", ArticleId: first.Id, ExpectedVersion: published.Article.Version})
	if err != nil || archived.GetArticle().GetStatus() != blogv1.ArticleStatus_ARTICLE_STATUS_ARCHIVED {
		t.Fatalf("archive=%+v err=%v", archived, err)
	}
	got, err := client.GetArticle(ctx, &blogv1.GetArticleRequest{UserId: operationUser, ArticleId: first.Id})
	if err != nil || got.GetArticle().GetTitle() != "grpc-patched" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	found, err := client.SearchArticles(ctx, &blogv1.SearchArticlesRequest{UserId: operationUser, Page: 1, PageSize: 1, Query: "grpc", Status: blogv1.ArticleStatus_ARTICLE_STATUS_ARCHIVED, ArticleTypeId: fixture.typeID, Sort: "title"})
	if err != nil || found.GetTotalItems() != 1 || len(found.GetArticles()) != 1 || found.GetTotalPages() != 1 {
		t.Fatalf("search=%+v err=%v", found, err)
	}
	drafts, err := client.SearchArticles(ctx, &blogv1.SearchArticlesRequest{UserId: operationUser, Status: blogv1.ArticleStatus_ARTICLE_STATUS_DRAFT})
	if err != nil || drafts.GetTotalItems() != 0 {
		t.Fatalf("status filter=%+v err=%v", drafts, err)
	}
	tags, err := client.ListTags(ctx, &blogv1.ListTagsRequest{UserId: operationUser, Query: "application-test-tag", Page: 1, PageSize: 1})
	if err != nil || tags.GetTotalItems() != 1 || tags.Tags[0].Id != fixture.tagID {
		t.Fatalf("tags=%+v err=%v", tags, err)
	}
	types, err := client.ListArticleTypes(ctx, &blogv1.ListArticleTypesRequest{UserId: operationUser, Query: "application-test-type", Page: 1, PageSize: 1})
	if err != nil || types.GetTotalItems() != 1 || types.ArticleTypes[0].Id != fixture.typeID {
		t.Fatalf("types=%+v err=%v", types, err)
	}
	_, err = client.CreateArticle(ctx, &blogv1.CreateArticleRequest{UserId: operationUser, OperationId: "not-a-uuid"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid key=%v", err)
	}
	_, err = client.PublishArticle(ctx, &blogv1.PublishArticleRequest{UserId: operationUser, OperationId: "50000000-0000-4000-8000-000000000006", ArticleId: first.Id, ExpectedVersion: first.Version})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("stale version=%v", err)
	}
	policy.denied.Store(true)
	_, err = client.ListTags(ctx, &blogv1.ListTagsRequest{UserId: operationUser})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("taxonomy permission=%v", err)
	}
	_, err = client.UpdateArticle(ctx, &blogv1.UpdateArticleRequest{UserId: operationUser, OperationId: key})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("replay permission=%v", err)
	}
	policy.denied.Store(false)
	if err := fixture.db.GORM().Exec(`UPDATE users SET is_active=false WHERE id=?`, operationUser).Error; err != nil {
		t.Fatal(err)
	}
	_, err = client.UpdateArticle(ctx, &blogv1.UpdateArticleRequest{UserId: operationUser, OperationId: key})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("inactive replay=%v", err)
	}
	_, err = client.GetArticle(ctx, &blogv1.GetArticleRequest{UserId: "22222222222222222222222222222222", ArticleId: first.Id})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unknown identity=%v", err)
	}
}
