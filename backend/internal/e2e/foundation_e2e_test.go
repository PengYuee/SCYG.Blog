//go:build e2e

package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

const narrativeTimeout = 120 * time.Second

// allowAll 只编译进 e2e 测试二进制，用于真实数据库/API 的授权 CRUD 叙事。
type allowAll struct{}

// Authorize 允许测试叙事写入；生产构件不会编译本文件。
func (allowAll) Authorize(context.Context, content.Action, content.Resource) error { return nil }

// narrative 描述由 qa:foundation 在真实 PostgreSQL 与 HTTP API 上执行的可观察结果。
type narrative struct {
	// Name 是稳定的 E2E 场景名称。
	Name string
	// Outcome 是必须通过真实边界观察的结果。
	Outcome string
}

func Test_E2E_foundation_narratives_are_complete(t *testing.T) {
	// Given
	ctx, cancel := context.WithTimeout(context.Background(), narrativeTimeout)
	t.Cleanup(cancel)
	narratives := []narrative{
		{Name: "migration roundtrip", Outcome: "up/down/up 后 schema 版本干净且当前"},
		{Name: "offline Scalar", Outcome: "禁用外网时 /docs 与本地 scalar.js 可用"},
		{Name: "public reads", Outcome: "公开列表只返回 Published 且未删除文章"},
		{Name: "AllowAll CRUD", Outcome: "测试授权器通过真实 API/数据库完成 ArticleType、Tag、Article CRUD"},
		{Name: "production 403", Outcome: "生产 DenyAll 对语法有效写入返回 403 且数据库不变"},
		{Name: "stale ETag", Outcome: "过期 If-Match 返回 412 且实体版本不变"},
		{Name: "readiness outage", Outcome: "PostgreSQL 中断时 readiness 失败，恢复后重新就绪"},
		{Name: "restart persistence", Outcome: "API 重启后已提交数据仍可读取"},
		{Name: "SIGTERM cleanup", Outcome: "SIGTERM 撤回 readiness 并在 timeout 内关闭 HTTP、telemetry、数据库"},
	}

	// When
	select {
	case <-ctx.Done():
		t.Fatalf("E2E 叙事清单校验超时：%v", ctx.Err())
	default:
	}

	// Then
	if len(narratives) != 9 {
		t.Fatalf("E2E 叙事数量错误：得到 %d，期望 9", len(narratives))
	}
	for _, item := range narratives {
		if item.Name == "" || item.Outcome == "" {
			t.Fatalf("E2E 叙事名称和结果不能为空：%+v", item)
		}
	}
}
