// Package blogcontent adapts internal Blog management capabilities to gRPC.
package blogcontent

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	blogv1 "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/blog/v1"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
)

// ArticleReader is the management projection consumed by this adapter.
type ArticleReader interface {
	ListManage(context.Context, article.List) (article.Page, error)
	GetManage(context.Context, article.Get) (article.Result, error)
}

// TaxonomyReader is the read-only taxonomy capability consumed by this adapter.
type TaxonomyReader interface {
	ListManageTags(context.Context, taxonomy.ListTags) (taxonomy.TagPage, error)
	ListManageArticleTypes(context.Context, taxonomy.ListArticleTypes) (taxonomy.ArticleTypePage, error)
	TagsByIDs(context.Context, []int64) ([]taxonomy.TagResult, error)
}

// ArticleWriter owns the four globally idempotent business commands.
type ArticleWriter interface {
	Create(context.Context, string, article.Create) (article.Result, error)
	Patch(context.Context, string, article.Patch) (article.Result, error)
	Publish(context.Context, string, article.Publish) (article.Result, error)
	Archive(context.Context, string, article.Archive) (article.Result, error)
}

// ActiveUsers resolves a declared internal user against Blog's account registry.
type ActiveUsers interface {
	FindActiveByID(context.Context, user.ID) (*user.User, error)
}

// Dependencies contains consumer-owned capabilities, never database handles.
type Dependencies struct {
	Articles   ArticleReader
	Taxonomy   TaxonomyReader
	Operations ArticleWriter
	Users      ActiveUsers
}

// Server exposes the eight Blog management RPCs to the trusted internal caller.
type Server struct {
	blogv1.UnimplementedBlogContentServiceServer
	dependencies Dependencies
}

// Register registers content and standard health without opening a listener.
// The lifecycle owner marks health serving only when startup is complete.
func Register(server *grpc.Server, dependencies Dependencies) (*health.Server, error) {
	if server == nil || dependencies.Articles == nil || dependencies.Taxonomy == nil || dependencies.Operations == nil || dependencies.Users == nil {
		return nil, errors.New("blog content server dependencies are incomplete")
	}
	blogv1.RegisterBlogContentServiceServer(server, &Server{dependencies: dependencies})
	checker := health.NewServer()
	checker.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	checker.SetServingStatus(blogv1.BlogContentService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_NOT_SERVING)
	healthpb.RegisterHealthServer(server, checker)
	return checker, nil
}

func (server *Server) businessContext(ctx context.Context, raw string) (context.Context, error) {
	id, err := user.ParseID(raw)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}
	account, err := server.dependencies.Users.FindActiveByID(ctx, id)
	if errors.Is(err, user.ErrNotFound) || (err == nil && (account == nil || !account.Active)) {
		return nil, status.Error(codes.Unauthenticated, "active user required")
	}
	if err != nil {
		return nil, rpcError(err)
	}
	// This principal is an internal business identity, not proof of JWT authentication.
	return identityauth.WithPrincipal(ctx, identityauth.Principal{UserID: id}), nil
}

func operationID(raw string) error {
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' || raw[14] != '4' || !strings.ContainsRune("89abAB", rune(raw[19])) {
		return status.Error(codes.InvalidArgument, "operation_id must be UUIDv4")
	}
	for i, c := range raw {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return status.Error(codes.InvalidArgument, "operation_id must be UUIDv4")
		}
	}
	return nil
}

func rpcError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "request canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "request deadline exceeded")
	}
	var stable interface{ StableCode() string }
	code := codes.Internal
	if errors.As(err, &stable) {
		switch stable.StableCode() {
		case "validation":
			code = codes.InvalidArgument
		case "permission_denied":
			code = codes.PermissionDenied
		case "not_found":
			code = codes.NotFound
		case "stale_version":
			code = codes.Aborted
		case "already_exists":
			code = codes.AlreadyExists
		case "failed_precondition", "conflict":
			code = codes.FailedPrecondition
		}
	}
	if code == codes.Internal {
		return status.Error(code, "internal error")
	}
	return status.Error(code, "business request rejected")
}
