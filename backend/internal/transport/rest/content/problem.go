package content

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
)

type stableFailure interface{ StableCode() string }

type restError struct {
	code  string
	cause error
}

func (failure *restError) Error() string {
	if failure == nil {
		return codeInternal
	}
	if failure.cause == nil {
		return failure.code
	}
	return failure.code + ": " + failure.cause.Error()
}

func (failure *restError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.cause
}

func (failure *restError) StableCode() string {
	if failure == nil {
		return codeInternal
	}
	return failure.code
}

func newRESTError(code string, cause error) error { return &restError{code: code, cause: cause} }

const (
	codeValidation         = "validation"
	codeUnauthenticated    = "unauthenticated"
	codePermissionDenied   = "permission_denied"
	codeNotFound           = "not_found"
	codeConflict           = "conflict"
	codeAlreadyExists      = "already_exists"
	codeFailedPrecondition = "failed_precondition"
	codeStaleVersion       = "stale_version"
	codeVersionRequired    = "version_required"
	codeInternal           = "internal"
)

func writeApplicationProblem(ctx *gin.Context, err error) {
	var failure stableFailure
	if !errors.As(err, &failure) || failure == nil {
		writeProblem(ctx, http.StatusInternalServerError, codeInternal, "服务器处理请求时发生内部错误", nil)
		return
	}
	code := failure.StableCode()
	status, detail := http.StatusInternalServerError, "服务器处理请求时发生内部错误"
	switch code {
	case codeValidation:
		status, detail = http.StatusBadRequest, "请求参数不合法"
	case codeUnauthenticated:
		status, detail = http.StatusUnauthorized, "用户名或密码错误"
	case codePermissionDenied:
		status, detail = http.StatusForbidden, "没有执行该操作的权限"
	case codeNotFound:
		status, detail = http.StatusNotFound, "请求的资源不存在"
	case codeConflict, codeAlreadyExists, codeFailedPrecondition:
		status, detail = http.StatusConflict, "操作与当前资源状态冲突"
	case codeStaleVersion:
		status, detail = http.StatusPreconditionFailed, "提供的实体版本已过期"
	case codeVersionRequired:
		status, detail = http.StatusPreconditionRequired, "必须提供强 If-Match 实体标签"
	default:
		code = codeInternal
	}
	writeProblem(ctx, status, code, detail, nil)
}

func writeProblem(ctx *gin.Context, status int, code, detail string, fields map[string][]string) {
	if fields == nil {
		fields = map[string][]string{}
	}
	statusCode := int32(http.StatusInternalServerError)
	if status >= 0 && status <= 1<<31-1 {
		statusCode = int32(status)
	}
	problem := generated.Problem{Type: "https://scyg.blog/problems/" + code, Title: problemTitle(status), Status: statusCode, Detail: detail, Instance: ctx.Request.URL.Path, RequestID: observability.RequestIDFromContext(ctx.Request.Context()), Errors: fields}
	ctx.Header("Content-Type", "application/problem+json")
	ctx.JSON(status, problem)
}

// problemTitle 返回 RFC 9457 响应使用的中文状态标题，避免向 HTTP 调用方暴露英文默认文案。
func problemTitle(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "请求参数错误"
	case http.StatusUnauthorized:
		return "身份认证失败"
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
