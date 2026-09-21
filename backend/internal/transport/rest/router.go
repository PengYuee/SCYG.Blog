// Package rest 组合当前 REST 传输、健康检查与离线文档路由。
package rest

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/image"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
	"github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/apidocs"
	contentrest "github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/content"
)

// Options 是当前 REST 传输所需的显式能力依赖。
type Options struct {
	ArticleQueries  contentrest.ArticleQueryService
	ArticleCommands contentrest.ArticleCommandService
	ArticleDeleter  contentrest.ArticleDeleteService
	Taxonomy        contentrest.TaxonomyService
	Images          contentrest.ArticleImageService
	ImagePolicy     image.Policy
	Health          *observability.Health
	DocsEnabled     bool
	TokenVerifier   TokenVerifier
	Login           *identityauth.LoginService
}

// New constructs the REST route mount with explicit health, login, and token capabilities.
func New(options Options) (func(*gin.Engine) error, error) {
	handler, err := contentrest.NewHandler(options.ArticleQueries, options.ArticleCommands, options.ArticleDeleter, options.Taxonomy, options.Images, options.ImagePolicy)
	if err != nil {
		return nil, err
	}
	loginHandler, err := NewLoginHandler(options.Login)
	if err != nil {
		return nil, err
	}
	if options.Health == nil {
		return nil, errors.New("健康检查为空")
	}
	return func(engine *gin.Engine) error {
		if options.TokenVerifier != nil {
			engine.Use(OptionalBearerAuthentication(options.TokenVerifier))
		}
		engine.Use(RequireAuthenticatedRoute())
		// 契约校验仅作用于生成路由，避免文档和健康端点被 OpenAPI 内容契约拦截。
		generatedRoutes := engine.Group("")
		if registerErr := handler.Register(generatedRoutes, loginHandler); registerErr != nil {
			return registerErr
		}
		engine.GET("/live", func(ctx *gin.Context) { ctx.JSON(http.StatusOK, gin.H{"message": "服务存活"}) })
		engine.GET("/ready", func(ctx *gin.Context) {
			ready, readyErr := options.Health.Ready(ctx.Request.Context())
			if !ready || readyErr != nil {
				ctx.JSON(http.StatusServiceUnavailable, gin.H{"message": "服务尚未就绪"})
				return
			}
			ctx.JSON(http.StatusOK, gin.H{"message": "服务已就绪"})
		})
		return apidocs.Mount(engine, options.DocsEnabled)
	}, nil
}
