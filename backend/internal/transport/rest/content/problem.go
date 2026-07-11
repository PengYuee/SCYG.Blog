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
	if !errors.As(err, &failure) {
		writeProblem(ctx, http.StatusInternalServerError, module.CodeInternal, "an internal error occurred", nil)
		return
	}
	status, detail := http.StatusInternalServerError, "an internal error occurred"
	switch failure.Code {
	case module.CodeValidation:
		status, detail = 400, "request values are invalid"
	case module.CodePermissionDenied:
		status, detail = 403, "the operation is forbidden"
	case module.CodeNotFound:
		status, detail = 404, "the requested resource was not found"
	case module.CodeAlreadyExists, module.CodeFailedPrecondition:
		status, detail = 409, "the operation conflicts with current state"
	case module.CodeStaleVersion:
		status, detail = 412, "the supplied entity version is stale"
	case module.CodeVersionRequired:
		status, detail = 428, "a strong If-Match entity tag is required"
	}
	writeProblem(ctx, status, failure.Code, detail, nil)
}

func writeProblem(ctx *gin.Context, status int, code module.ErrorCode, detail string, fields map[string][]string) {
	if fields == nil {
		fields = map[string][]string{}
	}
	problem := generated.Problem{Type: "https://scyg.blog/problems/" + string(code), Title: http.StatusText(status), Status: int32(status), Detail: detail, Instance: ctx.Request.URL.Path, RequestID: observability.RequestIDFromContext(ctx.Request.Context()), Errors: fields}
	ctx.Header("Content-Type", "application/problem+json")
	ctx.JSON(status, problem)
}
