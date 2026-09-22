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
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/image"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
	"github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/contract"
)

// ArticleQueryService 是公开文章读取 REST 方法持有的最小能力。
type ArticleQueryService interface {
	Get(context.Context, article.Get) (article.Result, error)
	List(context.Context, article.List) (article.Page, error)
	GetManage(context.Context, article.Get) (article.Result, error)
	ListManage(context.Context, article.List) (article.Page, error)
}

// ArticleCommandService 是文章创建、管理端 Patch 和状态迁移 REST 方法持有的最小能力。
type ArticleCommandService interface {
	Create(context.Context, article.Create) (article.Result, error)
	Patch(context.Context, article.Patch) (article.Result, error)
	Publish(context.Context, article.Publish) (article.Result, error)
	Archive(context.Context, article.Archive) (article.Result, error)
}

// ArticleDeleteService 是文章删除 REST 方法持有的最小能力。
type ArticleDeleteService interface {
	Delete(context.Context, article.Delete) error
}

// TaxonomyService 是分类和标签 REST 方法持有的最小能力。
type TaxonomyService interface {
	GetArticleType(context.Context, taxonomy.GetArticleType) (taxonomy.ArticleTypeResult, error)
	ListArticleTypes(context.Context, taxonomy.ListArticleTypes) (taxonomy.ArticleTypePage, error)
	GetPublicArticleType(context.Context, taxonomy.GetPublicArticleType) (taxonomy.PublicArticleTypeResult, error)
	ListPublicArticleTypes(context.Context, taxonomy.ListPublicArticleTypes) (taxonomy.PublicArticleTypePage, error)
	GetTag(context.Context, taxonomy.GetTag) (taxonomy.TagResult, error)
	ListTags(context.Context, taxonomy.ListTags) (taxonomy.TagPage, error)
	GetPublicTag(context.Context, taxonomy.GetPublicTag) (taxonomy.PublicTagResult, error)
	ListPublicTags(context.Context, taxonomy.ListPublicTags) (taxonomy.PublicTagPage, error)
	CreateArticleType(context.Context, taxonomy.CreateArticleType) (taxonomy.ArticleTypeResult, error)
	PatchArticleType(context.Context, taxonomy.PatchArticleType) (taxonomy.ArticleTypeResult, error)
	DeleteArticleType(context.Context, taxonomy.DeleteArticleType) error
	CreateTag(context.Context, taxonomy.CreateTag) (taxonomy.TagResult, error)
	RenameTag(context.Context, taxonomy.RenameTag) (taxonomy.TagResult, error)
	DeleteTag(context.Context, taxonomy.DeleteTag) error
}

// ArticleImageService 是图片 REST 方法持有的最小能力。
type ArticleImageService interface {
	Upload(context.Context, image.Upload) (image.Result, error)
	Cancel(context.Context, image.Delete) error
	GetMedia(context.Context, image.Get) (image.Media, error)
}

// Handler 将生成 DTO 适配为协议无关的内容服务。
type Handler struct {
	articleQueries  ArticleQueryService
	articleCommands ArticleCommandService
	articleDeleter  ArticleDeleteService
	taxonomy        TaxonomyService
	images          ArticleImageService
	imagePolicy     image.Policy
	tempFiles       requestTempOperations
}

// NewHandler 构造严格的生成式传输适配器；每个能力接缝均不得为 nil。
func NewHandler(articleQueries ArticleQueryService, articleCommands ArticleCommandService, articleDeleter ArticleDeleteService, taxonomy TaxonomyService, images ArticleImageService, imagePolicy image.Policy) (*Handler, error) {
	if nilService(articleQueries) || nilService(articleCommands) || nilService(articleDeleter) || nilService(taxonomy) || nilService(images) {
		return nil, errors.New("内容 REST 服务为空")
	}
	return &Handler{articleQueries: articleQueries, articleCommands: articleCommands, articleDeleter: articleDeleter, taxonomy: taxonomy, images: images, imagePolicy: imagePolicy, tempFiles: osRequestTempOperations{}}, nil
}

// LoginHandler is the narrow generated transport capability for authentication.
type LoginHandler interface {
	Login(context.Context, generated.LoginRequestObject) (generated.LoginResponseObject, error)
}

// Register mounts contract validation and all generated routes.
func (handler *Handler) Register(router gin.IRouter, login LoginHandler) error {
	if nilService(login) {
		return errors.New("登录 REST 服务为空")
	}
	validation, err := contract.Middleware(contract.Options{ErrorHandler: handler.ContractFailure})
	if err != nil {
		return err
	}
	// 先保留 image 字段是否出现的三态语义，再运行 OpenAPI 契约校验。
	router.Use(captureArticleTypeImagePatch(), validation)
	server := &aggregateHandler{Handler: handler, login: login}
	strict := generated.NewStrictHandlerWithOptions(server, nil, generated.StrictGinServerOptions{RequestErrorHandlerFunc: handler.requestError, HandlerErrorFunc: handler.applicationError, ResponseErrorHandlerFunc: handler.applicationError})
	generated.RegisterHandlers(router, strict)
	return nil
}

type aggregateHandler struct {
	*Handler
	login LoginHandler
}

func (handler *aggregateHandler) Login(ctx context.Context, request generated.LoginRequestObject) (generated.LoginResponseObject, error) {
	return handler.login.Login(ctx, request)
}

const articleTypeImageKey = "content.article_type.image"

type imagePatch struct {
	provided bool
	value    *string
}

func captureArticleTypeImagePatch() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// PATCH 的 image 需要区分省略、null 与字符串，生成 DTO 无法单独保留该信息。
		if ctx.Request.Method == "PATCH" && (strings.HasPrefix(ctx.Request.URL.Path, "/api/v1/article-types/") || strings.HasPrefix(ctx.Request.URL.Path, "/api/v1/manage/article-types/")) && ctx.Request.Body != nil {
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
	switch value.Kind() { //nolint:exhaustive // only nullable kinds can be typed-nil.
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
	writeProblem(ctx, status, codeValidation, "请求不符合 API 契约", nil)
	ctx.Abort()
}

func (handler *Handler) requestError(ctx *gin.Context, _ error) {
	writeProblem(ctx, http.StatusBadRequest, codeValidation, "请求体格式不合法", nil)
}

func (handler *Handler) applicationError(ctx *gin.Context, err error) {
	writeApplicationProblem(ctx, err)
}

var _ generated.StrictServerInterface = (*aggregateHandler)(nil)
