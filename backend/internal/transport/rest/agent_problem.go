package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/agent/v1"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
)

// WriteAgentProblem writes the shared RFC 9457 envelope for Agent transport failures.
func WriteAgentProblem(ctx *gin.Context, httpStatus int, detail, code string) {
	if ctx.IsAborted() {
		return
	}
	ctx.Abort()
	ctx.Header("Content-Type", "application/problem+json")
	value := gin.H{"type": "https://scyg.blog/problems/agent", "title": http.StatusText(httpStatus), "status": httpStatus, "detail": detail, "instance": ctx.Request.URL.Path, "requestId": observability.RequestIDFromContext(ctx.Request.Context()), "errors": map[string][]string{}}
	if code != "" {
		value["code"] = code
	}
	ctx.JSON(httpStatus, value)
}

// AgentRequestTooLarge is injected into the platform request limiter without a reverse dependency.
func AgentRequestTooLarge(ctx *gin.Context) {
	WriteAgentProblem(ctx, http.StatusRequestEntityTooLarge, "请求体超过 1 MiB 限制", "")
}

func agentFailure(ctx *gin.Context, err error) {
	s := status.Convert(err)
	httpStatus := http.StatusInternalServerError
	switch s.Code() {
	case codes.InvalidArgument:
		httpStatus = 400
	case codes.Unauthenticated:
		httpStatus = 401
	case codes.NotFound, codes.PermissionDenied:
		httpStatus = 404
	case codes.AlreadyExists, codes.FailedPrecondition, codes.Aborted:
		httpStatus = 409
	case codes.ResourceExhausted:
		httpStatus = 429
	case codes.DeadlineExceeded, codes.Unavailable:
		httpStatus = 503
	case codes.OK, codes.Canceled, codes.Unknown, codes.OutOfRange, codes.Unimplemented, codes.Internal, codes.DataLoss:
		httpStatus = http.StatusInternalServerError
	}
	detail := "Agent 请求失败"
	code := ""
	for _, item := range s.Details() {
		if public, ok := item.(*pb.PublicError); ok {
			detail = public.Message
			code = public.Code
			break
		}
	}
	WriteAgentProblem(ctx, httpStatus, detail, code)
}
