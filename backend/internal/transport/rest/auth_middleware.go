package rest

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/authentication"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
)

// TokenVerifier is the authentication boundary required by the HTTP middleware.
type TokenVerifier interface {
	Verify(string) (identityauth.Principal, error)
}

// AnonymousVerifier verifies only the public-search credential domain.
type AnonymousVerifier interface {
	Verify(string) (string, error)
}

// OptionalBearerAuthentication authenticates requests that provide a Bearer token.
// Public routes do not require or inspect credentials; protected routes reject a
// supplied malformed or invalid token before authorization runs.
func OptionalBearerAuthentication(verifier TokenVerifier, anonymous ...AnonymousVerifier) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if isPublicRoute(ctx) {
			ctx.Next()
			return
		}
		header := strings.TrimSpace(ctx.GetHeader("Authorization"))
		if header == "" {
			ctx.Next()
			return
		}
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			abortAuthentication(ctx, "认证凭据格式无效")
			return
		}
		publicOnly := ctx.FullPath() == "/api/v1/public/ai/search"
		dual := strings.HasPrefix(ctx.FullPath(), "/api/v1/runs/") && !strings.HasSuffix(ctx.FullPath(), "/resume")
		if (publicOnly || dual) && len(anonymous) == 1 && anonymous[0] != nil {
			if subject, err := anonymous[0].Verify(parts[1]); err == nil {
				ctx.Request = ctx.Request.WithContext(authentication.WithAnonymousSubject(ctx.Request.Context(), subject))
				ctx.Next()
				return
			}
		}
		if publicOnly || verifier == nil {
			abortAuthentication(ctx, "认证凭据无效")
			return
		}
		principal, err := verifier.Verify(parts[1])
		if err != nil {
			abortAuthentication(ctx, "认证凭据无效")
			return
		}
		ctx.Request = ctx.Request.WithContext(identityauth.WithPrincipal(ctx.Request.Context(), principal))
		ctx.Next()
	}
}

// RequireAuthenticatedRoute enforces the OpenAPI public/protected route split.
func RequireAuthenticatedRoute() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if isPublicRoute(ctx) {
			ctx.Next()
			return
		}
		_, author := identityauth.PrincipalFromContext(ctx.Request.Context())
		_, anonymous := authentication.AnonymousSubject(ctx.Request.Context())
		if !author && !anonymous {
			abortAuthentication(ctx, "需要有效的 Bearer 认证凭据")
			return
		}
		ctx.Next()
	}
}

func isPublicRoute(ctx *gin.Context) bool {
	// Disabled Agent endpoints are unregistered and must retain the router's 404.
	if ctx.FullPath() == "" && (strings.HasPrefix(ctx.Request.URL.Path, "/api/v1/ai/") || strings.HasPrefix(ctx.Request.URL.Path, "/api/v1/runs/") || strings.HasPrefix(ctx.Request.URL.Path, "/api/v1/public/ai/")) {
		return true
	}
	if ctx.Request.Method == http.MethodPost && (ctx.FullPath() == "/api/v1/auth/login" || ctx.FullPath() == "/api/v1/public/ai/session") {
		return true
	}
	if ctx.Request.Method != http.MethodGet {
		return false
	}
	switch ctx.FullPath() {
	case "/api/v1/articles", "/api/v1/articles/:articleId",
		"/api/v1/article-types", "/api/v1/article-types/:articleTypeId",
		"/api/v1/tags", "/api/v1/tags/:tagId",
		"/media/article-images/:storageKey":
		return true
	default:
		path := ctx.Request.URL.Path
		return ctx.FullPath() == "/live" || ctx.FullPath() == "/ready" || ctx.FullPath() == "/openapi.yaml" || path == "/docs" || path == "/docs/assets/scalar.js"
	}
}

func abortAuthentication(ctx *gin.Context, detail string) {
	problem := generated.Problem{
		Type:      "https://scyg.blog/problems/unauthenticated",
		Title:     "身份认证失败",
		Status:    http.StatusUnauthorized,
		Detail:    detail,
		Instance:  ctx.Request.URL.Path,
		RequestID: observability.RequestIDFromContext(ctx.Request.Context()),
		Errors:    map[string][]string{},
	}
	ctx.Abort()
	ctx.Header("Content-Type", "application/problem+json")
	ctx.JSON(http.StatusUnauthorized, problem)
}
