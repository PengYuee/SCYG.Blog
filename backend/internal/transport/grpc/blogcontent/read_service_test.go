package blogcontent

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	agentv1 "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/agent/v1"
	blogv1 "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/blog/v1"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
)

const anonymousSubject = "anonymous:12345678-1234-4234-8234-123456789abc"

type scopedArticleReader struct {
	t                        *testing.T
	publicCalls, authorCalls int
	query                    article.List
}

func (reader *scopedArticleReader) result(ctx context.Context, public bool) article.Result {
	reader.t.Helper()
	_, principal := identityauth.PrincipalFromContext(ctx)
	if principal == public {
		reader.t.Fatalf("public=%t author principal=%t", public, principal)
	}
	state := "draft"
	if public {
		reader.publicCalls++
		state = "published"
	} else {
		reader.authorCalls++
	}
	return article.Result{ID: 1, Status: state}
}

func (reader *scopedArticleReader) List(ctx context.Context, query article.List) (article.Page, error) {
	reader.query = query
	return article.Page{Items: []article.Result{reader.result(ctx, true)}, Number: 1, Size: query.PageSize, TotalItems: 1, TotalPages: 1}, nil
}

func (reader *scopedArticleReader) Get(ctx context.Context, _ article.Get) (article.Result, error) {
	return reader.result(ctx, true), nil
}

func (reader *scopedArticleReader) ListManage(ctx context.Context, query article.List) (article.Page, error) {
	reader.query = query
	return article.Page{Items: []article.Result{reader.result(ctx, false)}, Number: 1, Size: query.PageSize, TotalItems: 1, TotalPages: 1}, nil
}

func (reader *scopedArticleReader) GetManage(ctx context.Context, _ article.Get) (article.Result, error) {
	return reader.result(ctx, false), nil
}

type scopedTaxonomyReader struct{}

func (scopedTaxonomyReader) TagsByIDs(context.Context, []int64) ([]taxonomy.TagResult, error) {
	return nil, nil
}

func (scopedTaxonomyReader) ListManageTags(context.Context, taxonomy.ListTags) (taxonomy.TagPage, error) {
	panic("anonymous request reached taxonomy management")
}

func (scopedTaxonomyReader) ListManageArticleTypes(context.Context, taxonomy.ListArticleTypes) (taxonomy.ArticleTypePage, error) {
	panic("anonymous request reached taxonomy management")
}

func TestContentScopeDispatchPreservesIdentityBoundary(t *testing.T) {
	id, err := user.ParseID("abcdef0123456789abcdef0123456789")
	if err != nil {
		t.Fatal(err)
	}
	account, err := user.Reconstitute(id, "scope-author", "stored-test-hash", true, time.Now().UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	reader := &scopedArticleReader{t: t}
	server := &Server{dependencies: Dependencies{Articles: reader, Taxonomy: scopedTaxonomyReader{}, Users: testAccounts{account: account}}}
	ctx := context.Background()
	for _, scope := range []agentv1.ContentScope{agentv1.ContentScope_CONTENT_SCOPE_PUBLIC, agentv1.ContentScope_CONTENT_SCOPE_AUTHOR} {
		subject := anonymousSubject
		want := blogv1.ArticleStatus_ARTICLE_STATUS_PUBLISHED
		if scope == agentv1.ContentScope_CONTENT_SCOPE_AUTHOR {
			subject = id.String()
			want = blogv1.ArticleStatus_ARTICLE_STATUS_DRAFT
		}
		found, err := server.SearchArticles(ctx, &blogv1.SearchArticlesRequest{UserId: subject, ContentScope: scope, Query: "needle", Page: 1, PageSize: 3, ArticleTypeId: 4, TagId: 5, Sort: "title"})
		if err != nil || len(found.GetArticles()) != 1 || found.Articles[0].Status != want || found.TotalItems != 1 {
			t.Fatalf("scope=%v search=%+v err=%v", scope, found, err)
		}
		if reader.query.Query != "needle" || reader.query.PageSize != 3 || reader.query.ArticleTypeID != 4 || reader.query.TagID != 5 || reader.query.Sort != "title" {
			t.Fatalf("lost search filters: %+v", reader.query)
		}
		got, err := server.GetArticle(ctx, &blogv1.GetArticleRequest{UserId: subject, ContentScope: scope, ArticleId: 1})
		if err != nil || got.GetArticle().GetStatus() != want {
			t.Fatalf("scope=%v get=%+v err=%v", scope, got, err)
		}
	}
	if reader.publicCalls != 2 || reader.authorCalls != 2 {
		t.Fatalf("public=%d author=%d", reader.publicCalls, reader.authorCalls)
	}
	account.Active = false
	if _, err := server.GetArticle(ctx, &blogv1.GetArticleRequest{UserId: id.String(), ContentScope: agentv1.ContentScope_CONTENT_SCOPE_AUTHOR}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("inactive author=%v", err)
	}
}

func TestContentScopeRejectsMissingUnknownAndInvalidPublicSubjects(t *testing.T) {
	server := &Server{}
	ctx := context.Background()
	for _, scope := range []agentv1.ContentScope{agentv1.ContentScope_CONTENT_SCOPE_UNSPECIFIED, agentv1.ContentScope(99), agentv1.ContentScope(-1)} {
		if _, err := server.SearchArticles(ctx, &blogv1.SearchArticlesRequest{UserId: anonymousSubject, ContentScope: scope}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("search scope=%v err=%v", scope, err)
		}
		if _, err := server.GetArticle(ctx, &blogv1.GetArticleRequest{UserId: anonymousSubject, ContentScope: scope}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("get scope=%v err=%v", scope, err)
		}
	}
	for _, subject := range []string{"", "abcdef0123456789abcdef0123456789", "anonymous:", "anonymous:12345678-1234-1234-8234-123456789abc", anonymousSubject + "\n"} {
		if _, err := server.GetArticle(ctx, &blogv1.GetArticleRequest{UserId: subject, ContentScope: agentv1.ContentScope_CONTENT_SCOPE_PUBLIC}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("subject=%q err=%v", subject, err)
		}
	}
	for _, state := range []blogv1.ArticleStatus{blogv1.ArticleStatus_ARTICLE_STATUS_DRAFT, blogv1.ArticleStatus_ARTICLE_STATUS_ARCHIVED, blogv1.ArticleStatus(99)} {
		if _, err := server.SearchArticles(ctx, &blogv1.SearchArticlesRequest{UserId: anonymousSubject, ContentScope: agentv1.ContentScope_CONTENT_SCOPE_PUBLIC, Status: state}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("public status=%v err=%v", state, err)
		}
	}
	if _, err := publicContext(identityauth.WithPrincipal(ctx, identityauth.Principal{}), anonymousSubject); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("public inherited author principal=%v", err)
	}
}

func TestAnonymousSubjectCannotAccessManagementRPCs(t *testing.T) {
	server := &Server{}
	ctx := context.Background()
	checks := []struct {
		name string
		call func() error
	}{
		{"author search", func() error {
			_, err := server.SearchArticles(ctx, &blogv1.SearchArticlesRequest{UserId: anonymousSubject, ContentScope: agentv1.ContentScope_CONTENT_SCOPE_AUTHOR})
			return err
		}},
		{"author get", func() error {
			_, err := server.GetArticle(ctx, &blogv1.GetArticleRequest{UserId: anonymousSubject, ContentScope: agentv1.ContentScope_CONTENT_SCOPE_AUTHOR})
			return err
		}},
		{"tags", func() error {
			_, err := server.ListTags(ctx, &blogv1.ListTagsRequest{UserId: anonymousSubject})
			return err
		}},
		{"types", func() error {
			_, err := server.ListArticleTypes(ctx, &blogv1.ListArticleTypesRequest{UserId: anonymousSubject})
			return err
		}},
		{"create", func() error {
			_, err := server.CreateArticle(ctx, &blogv1.CreateArticleRequest{UserId: anonymousSubject})
			return err
		}},
		{"update", func() error {
			_, err := server.UpdateArticle(ctx, &blogv1.UpdateArticleRequest{UserId: anonymousSubject})
			return err
		}},
		{"publish", func() error {
			_, err := server.PublishArticle(ctx, &blogv1.PublishArticleRequest{UserId: anonymousSubject})
			return err
		}},
		{"archive", func() error {
			_, err := server.ArchiveArticle(ctx, &blogv1.ArchiveArticleRequest{UserId: anonymousSubject})
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("anonymous management access=%v", err)
			}
		})
	}
}
