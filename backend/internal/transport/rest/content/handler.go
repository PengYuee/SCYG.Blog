// Package content owns the generated Gin transport adapter for content use cases.
package content

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/contract"
)

// QueryService 是传输层持有的只读服务面。
type QueryService interface {
	GetArticle(context.Context, module.GetArticle) (module.ArticleResult, error)
	ListArticles(context.Context, module.ListArticles) (module.ArticlePage, error)
	GetArticleType(context.Context, module.GetArticleType) (module.ArticleTypeResult, error)
	ListArticleTypes(context.Context, module.ListArticleTypes) (module.ArticleTypePage, error)
	GetTag(context.Context, module.GetTag) (module.TagResult, error)
	ListTags(context.Context, module.ListTags) (module.TagPage, error)
}

// CommandService 是传输层持有的写入服务面。
type CommandService interface {
	CreateArticle(context.Context, module.CreateArticle) (module.ArticleResult, error)
	PatchArticle(context.Context, module.PatchArticle) (module.ArticleResult, error)
	DeleteArticle(context.Context, module.DeleteArticle) error
	CreateArticleType(context.Context, module.CreateArticleType) (module.ArticleTypeResult, error)
	PatchArticleType(context.Context, module.PatchArticleType) (module.ArticleTypeResult, error)
	DeleteArticleType(context.Context, module.DeleteArticleType) error
	CreateTag(context.Context, module.CreateTag) (module.TagResult, error)
	RenameTag(context.Context, module.RenameTag) (module.TagResult, error)
	DeleteTag(context.Context, module.DeleteTag) error
}

// Handler 将生成 DTO 适配为协议无关的内容服务。
type Handler struct {
	queries   QueryService
	commands  CommandService
	tempFiles requestTempOperations
}

// NewHandler 构造严格的生成式传输适配器；读写服务均不得为 nil。
func NewHandler(queries QueryService, commands CommandService) (*Handler, error) {
	if nilService(queries) || nilService(commands) {
		return nil, errors.New("内容 REST 服务为空")
	}
	return &Handler{queries: queries, commands: commands, tempFiles: osRequestTempOperations{}}, nil
}

// Register 挂载契约校验和全部生成式严格内容路由。
func (handler *Handler) Register(router gin.IRouter) error {
	validation, err := contract.Middleware(contract.Options{ErrorHandler: handler.ContractFailure})
	if err != nil {
		return err
	}
	// 先保留 image 字段是否出现的三态语义，再运行 OpenAPI 契约校验。
	router.Use(captureArticleTypeImagePatch(), validation)
	strict := generated.NewStrictHandlerWithOptions(handler, nil, generated.StrictGinServerOptions{RequestErrorHandlerFunc: handler.requestError, HandlerErrorFunc: handler.applicationError, ResponseErrorHandlerFunc: handler.applicationError})
	generated.RegisterHandlers(router, strict)
	return nil
}

const articleTypeImageKey = "content.article_type.image"

type imagePatch struct {
	provided bool
	value    *string
}

func captureArticleTypeImagePatch() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// PATCH 的 image 需要区分省略、null 与字符串，生成 DTO 无法单独保留该信息。
		if ctx.Request.Method == "PATCH" && strings.HasPrefix(ctx.Request.URL.Path, "/api/v1/article-types/") && ctx.Request.Body != nil {
			body, err := io.ReadAll(ctx.Request.Body)
			if err == nil {
				ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
				var object map[string]json.RawMessage
				if json.Unmarshal(body, &object) == nil {
					if raw, exists := object["image"]; exists {
						patch := imagePatch{provided: true}
						if string(raw) != "null" {
							var value string
							if json.Unmarshal(raw, &value) == nil {
								patch.value = &value
							}
						}
						ctx.Set(articleTypeImageKey, patch)
					}
				}
			}
		}
		ctx.Next()
	}
}

func nilService(service any) bool {
	if service == nil {
		return true
	}
	value := reflect.ValueOf(service)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// ContractFailure 将契约校验失败映射为中文 RFC 9457 响应。
func (handler *Handler) ContractFailure(ctx *gin.Context, failure contract.Failure) {
	status := failure.Status
	if failure.Kind == contract.FailureVersionRequired {
		status = 428
	}
	writeProblem(ctx, status, module.CodeValidation, "请求不符合 API 契约", nil)
	ctx.Abort()
}

func (handler *Handler) requestError(ctx *gin.Context, _ error) {
	writeProblem(ctx, http.StatusBadRequest, module.CodeValidation, "请求体格式不合法", nil)
}

func (handler *Handler) applicationError(ctx *gin.Context, err error) {
	writeApplicationProblem(ctx, err)
}

var (
	_ generated.StrictServerInterface = (*Handler)(nil)
	_ QueryService                    = (*module.Module)(nil)
	_ CommandService                  = (*module.Module)(nil)
)
