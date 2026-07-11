package content

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
)

func writeApplicationProblem(ctx *gin.Context, err error) {
	var failure *module.ApplicationError
	if !errors.As(err, &failure) || failure == nil {
		writeProblem(ctx, http.StatusInternalServerError, module.CodeInternal, "服务器处理请求时发生内部错误", nil)
		return
	}
	status, detail := http.StatusInternalServerError, "服务器处理请求时发生内部错误"
	switch failure.Code {
	case module.CodeValidation:
		status, detail = http.StatusBadRequest, "请求参数不合法"
	case module.CodePermissionDenied:
		status, detail = http.StatusForbidden, "没有执行该操作的权限"
	case module.CodeNotFound:
		status, detail = http.StatusNotFound, "请求的资源不存在"
	case module.CodeAlreadyExists, module.CodeFailedPrecondition:
		status, detail = http.StatusConflict, "操作与当前资源状态冲突"
	case module.CodeStaleVersion:
		status, detail = http.StatusPreconditionFailed, "提供的实体版本已过期"
	case module.CodeVersionRequired:
		status, detail = http.StatusPreconditionRequired, "必须提供强 If-Match 实体标签"
	}
	writeProblem(ctx, status, failure.Code, detail, nil)
}

func writeProblem(ctx *gin.Context, status int, code module.ErrorCode, detail string, fields map[string][]string) {
	if fields == nil {
		fields = map[string][]string{}
	}
	problem := generated.Problem{Type: "https://scyg.blog/problems/" + string(code), Title: problemTitle(status), Status: int32(status), Detail: detail, Instance: ctx.Request.URL.Path, RequestID: observability.RequestIDFromContext(ctx.Request.Context()), Errors: fields}
	ctx.Header("Content-Type", "application/problem+json")
	ctx.JSON(status, problem)
}

// problemTitle 返回 RFC 9457 响应使用的中文状态标题，避免向 HTTP 调用方暴露英文默认文案。
func problemTitle(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "请求参数错误"
	case http.StatusForbidden:
		return "禁止访问"
	case http.StatusNotFound:
		return "资源不存在"
	case http.StatusConflict:
		return "资源状态冲突"
	case http.StatusPreconditionFailed:
		return "前置条件不满足"
	case http.StatusPreconditionRequired:
		return "缺少前置条件"
	default:
		return "服务器内部错误"
	}
}
