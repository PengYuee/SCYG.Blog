// Package content owns the generated Gin transport adapter for content use cases.
package content

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/contract"
)

// QueryService is the transport-owned read surface.
type QueryService interface {
	GetArticle(context.Context, module.GetArticle) (module.ArticleResult, error)
	ListArticles(context.Context, module.ListArticles) (module.ArticlePage, error)
	GetArticleType(context.Context, module.GetArticleType) (module.ArticleTypeResult, error)
	ListArticleTypes(context.Context, module.ListArticleTypes) (module.ArticleTypePage, error)
	GetTag(context.Context, module.GetTag) (module.TagResult, error)
	ListTags(context.Context, module.ListTags) (module.TagPage, error)
}

// CommandService is the transport-owned write surface.
type CommandService interface {
	CreateArticle(context.Context, module.CreateArticle) (module.ArticleResult, error)
	PatchArticle(context.Context, module.PatchArticle) (module.ArticleResult, error)
	DeleteArticle(context.Context, module.DeleteArticle) error
	CreateArticleType(context.Context, module.CreateArticleType) (module.ArticleTypeResult, error)
	RenameArticleType(context.Context, module.RenameArticleType) (module.ArticleTypeResult, error)
	DeleteArticleType(context.Context, module.DeleteArticleType) error
	CreateTag(context.Context, module.CreateTag) (module.TagResult, error)
	RenameTag(context.Context, module.RenameTag) (module.TagResult, error)
	DeleteTag(context.Context, module.DeleteTag) error
}

// Handler adapts generated DTOs to protocol-neutral content services.
type Handler struct {
	queries  QueryService
	commands CommandService
}

// NewHandler constructs the strict generated transport adapter.
func NewHandler(queries QueryService, commands CommandService) (*Handler, error) {
	if queries == nil || commands == nil {
		return nil, errors.New("content REST service is nil")
	}
	return &Handler{queries: queries, commands: commands}, nil
}

// Register mounts contract validation and all generated strict content routes.
func (handler *Handler) Register(router gin.IRouter) error {
	validation, err := contract.Middleware(contract.Options{ErrorHandler: handler.ContractFailure})
	if err != nil {
		return err
	}
	router.Use(validation)
	strict := generated.NewStrictHandlerWithOptions(handler, nil, generated.StrictGinServerOptions{RequestErrorHandlerFunc: handler.requestError, HandlerErrorFunc: handler.applicationError, ResponseErrorHandlerFunc: handler.applicationError})
	generated.RegisterHandlers(router, strict)
	return nil
}

// ContractFailure maps schema failures to RFC 9457 responses.
func (handler *Handler) ContractFailure(ctx *gin.Context, failure contract.Failure) {
	status := failure.Status
	if failure.Kind == contract.FailureVersionRequired {
		status = 428
	}
	writeProblem(ctx, status, module.CodeValidation, "request does not satisfy the API contract", nil)
	ctx.Abort()
}

func (handler *Handler) requestError(ctx *gin.Context, _ error) {
	writeProblem(ctx, 400, module.CodeValidation, "request body is invalid", nil)
}
func (handler *Handler) applicationError(ctx *gin.Context, err error) {
	writeApplicationProblem(ctx, err)
}

var _ generated.StrictServerInterface = (*Handler)(nil)
var _ QueryService = (*module.Module)(nil)
var _ CommandService = (*module.Module)(nil)
