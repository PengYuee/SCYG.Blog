package content

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func TestWriteApplicationProblem_usesChineseProblemTitlesAndDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		err    error
		status int
		title  string
		detail string
	}{
		{"校验失败", &module.ApplicationError{Code: module.CodeValidation}, http.StatusBadRequest, "请求参数错误", "请求参数不合法"},
		{"权限不足", &module.ApplicationError{Code: module.CodePermissionDenied}, http.StatusForbidden, "禁止访问", "没有执行该操作的权限"},
		{"资源不存在", &module.ApplicationError{Code: module.CodeNotFound}, http.StatusNotFound, "资源不存在", "请求的资源不存在"},
		{"状态冲突", &module.ApplicationError{Code: module.CodeAlreadyExists}, http.StatusConflict, "资源状态冲突", "操作与当前资源状态冲突"},
		{"版本过期", &module.ApplicationError{Code: module.CodeStaleVersion}, http.StatusPreconditionFailed, "前置条件不满足", "提供的实体版本已过期"},
		{"缺少版本", &module.ApplicationError{Code: module.CodeVersionRequired}, http.StatusPreconditionRequired, "缺少前置条件", "必须提供强 If-Match 实体标签"},
		{"未知错误", errors.New("数据库连接失败"), http.StatusInternalServerError, "服务器内部错误", "服务器处理请求时发生内部错误"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// 前置条件：构造纯内存的 Gin 请求上下文。
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/articles/1", nil)

			// 执行：将应用错误映射为 RFC 9457 响应。
			writeApplicationProblem(ctx, testCase.err)

			// 断言：状态码、标题和详情均为调用方可见的中文文案。
			var problem generated.Problem
			if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
				t.Fatalf("解析 problem 响应失败: %v", err)
			}
			if recorder.Code != testCase.status || problem.Title != testCase.title || problem.Detail != testCase.detail {
				t.Fatalf("problem = %#v, status = %d", problem, recorder.Code)
			}
			if strings.Contains(problem.Title, "Bad Request") || strings.Contains(problem.Title, "Internal Server Error") || strings.Contains(problem.Detail, "internal error") {
				t.Fatalf("problem 包含英文默认文案: %#v", problem)
			}
		})
	}
}
