// Package rest 组合当前 REST 传输、健康检查与离线文档路由。
package rest

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
	"github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/apidocs"
	contentrest "github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/content"
)

// Options 是当前 REST 传输所需的显式依赖。
type Options struct {
	// Content 提供协议无关的内容查询与命令。
	Content *module.Module
	// Health 提供存活和就绪状态。
	Health *observability.Health
	// DocsEnabled 决定是否挂载离线 API 文档。
	DocsEnabled bool
}

// New 构造一次性路由挂载函数；生成协议类型仅留在内容 REST 构造器内部。
func New(options Options) (func(*gin.Engine) error, error) {
	handler, err := contentrest.NewHandler(options.Content, options.Content)
	if err != nil {
		return nil, err
	}
	if options.Health == nil {
		return nil, errors.New("健康检查为空")
	}
	return func(engine *gin.Engine) error {
		// 契约校验仅作用于生成路由，避免文档和健康端点被 OpenAPI 内容契约拦截。
		generatedRoutes := engine.Group("")
		if registerErr := handler.Register(generatedRoutes); registerErr != nil {
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
