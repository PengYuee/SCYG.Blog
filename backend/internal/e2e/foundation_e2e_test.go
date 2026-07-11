//go:build e2e

package e2e_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func Test_E2E_migrations_roundtrip(t *testing.T) {
	h := newHarness(t, allowAll{})
	defer h.close()
	if err := h.app.Shutdown(h.ctx); err != nil {
		t.Fatalf("迁移叙事关闭应用失败：%v", err)
	}
	h.app = nil
	h.migrateDown()
	h.migrateUp()
	pool := openPool(t, h.dsn)
	defer pool.Close()
	var version int
	if err := pool.QueryRowContext(h.ctx, `SELECT version FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("读取迁移版本失败：%v", err)
	}
	if version != 1 {
		t.Fatalf("迁移版本错误：%d", version)
	}
}

func Test_E2E_scalar_is_offline_and_self_hosted(t *testing.T) {
	h := newHarness(t, nil)
	defer h.close()
	response := h.request(http.MethodGet, "/docs", "", nil)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("读取 Scalar 页面失败：%v", err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "/scalar.js") {
		t.Fatalf("Scalar 未使用本地资源：status=%d", response.StatusCode)
	}
}

func Test_E2E_public_reads_hide_drafts(t *testing.T) {
	h := newHarness(t, allowAll{})
	defer h.close()
	articleType, tag := createContent(t, h)
	draft := createArticle(t, h, articleType, tag, "draft", "draft")
	published := createArticle(t, h, articleType, tag, "published", "published")
	publish := h.request(http.MethodPatch, published.Header.Get("Location"), "{\"status\":\"published\"}", map[string]string{"If-Match": published.Header.Get("ETag")})
	if publish.StatusCode != http.StatusOK {
		t.Fatalf("发布文章失败：%d", publish.StatusCode)
	}
	response := h.request(http.MethodGet, "/api/v1/articles", "", nil)
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || strings.Contains(string(body), "e2e-draft") || !strings.Contains(string(body), "e2e-published") {
		t.Fatalf("公开列表可见性错误：%s", body)
	}
	if hidden := h.request(http.MethodGet, draft.Header.Get("Location"), "", nil); hidden.StatusCode != http.StatusNotFound {
		t.Fatalf("公开详情暴露草稿：%d", hidden.StatusCode)
	}
	if visible := h.request(http.MethodGet, published.Header.Get("Location"), "", nil); visible.StatusCode != http.StatusOK {
		t.Fatalf("公开详情未返回已发布文章：%d", visible.StatusCode)
	}
}

func Test_E2E_allow_all_performs_real_crud(t *testing.T) {
	h := newHarness(t, allowAll{})
	defer h.close()
	articleType, tag := createContent(t, h)
	article := createArticle(t, h, articleType, tag, "crud", "draft")
	for _, location := range []string{articleType.Header.Get("Location"), tag.Header.Get("Location"), article.Header.Get("Location")} {
		if got := h.request(http.MethodGet, location, "", nil); got.StatusCode != http.StatusOK && location != article.Header.Get("Location") {
			t.Fatalf("读取已创建资源失败：%s status=%d", location, got.StatusCode)
		}
	}
	articlePatch := h.request(http.MethodPatch, article.Header.Get("Location"), "{\"title\":\"e2e-crud-updated\"}", map[string]string{"If-Match": article.Header.Get("ETag")})
	if articlePatch.StatusCode != http.StatusOK {
		t.Fatalf("更新 Article 失败：%d", articlePatch.StatusCode)
	}
	if deleted := h.request(http.MethodDelete, article.Header.Get("Location"), "", map[string]string{"If-Match": articlePatch.Header.Get("ETag")}); deleted.StatusCode != http.StatusNoContent {
		t.Fatalf("删除 Article 失败：%d", deleted.StatusCode)
	}
	for _, resource := range []*http.Response{tag, articleType} {
		if deleted := h.request(http.MethodDelete, resource.Header.Get("Location"), "", map[string]string{"If-Match": resource.Header.Get("ETag")}); deleted.StatusCode != http.StatusNoContent {
			t.Fatalf("删除 taxonomy 失败：%d", deleted.StatusCode)
		}
	}
}

func Test_E2E_production_denies_writes(t *testing.T) {
	h := newHarness(t, nil)
	defer h.close()
	response := h.request(http.MethodPost, "/api/v1/tags", `{"name":"denied"}`, nil)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("生产写入未返回403：%d", response.StatusCode)
	}
}

func Test_E2E_stale_etag_is_rejected(t *testing.T) {
	h := newHarness(t, allowAll{})
	defer h.close()
	_, tag := createContent(t, h)
	location := tag.Header.Get("Location")
	response := h.request(http.MethodPatch, location, `{"name":"stale"}`, map[string]string{"If-Match": `"0"`})
	if response.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("过期 ETag 未返回412：%d", response.StatusCode)
	}
}

func Test_E2E_readiness_fails_during_database_outage(t *testing.T) {
	h := newHarness(t, nil)
	defer h.close()
	pool := openPool(t, h.adminDSN)
	defer pool.Close()
	if _, err := pool.ExecContext(h.ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, h.name); err != nil {
		t.Fatalf("中断数据库连接失败：%v", err)
	}
	response := h.request(http.MethodGet, "/ready", "", nil)
	if response.StatusCode == http.StatusOK {
		t.Fatal("数据库故障期间 readiness 仍为成功")
	}
}

func Test_E2E_restart_preserves_committed_data(t *testing.T) {
	h := newHarness(t, allowAll{})
	defer h.close()
	_, _ = createContent(t, h)
	if err := h.app.Shutdown(h.ctx); err != nil {
		t.Fatalf("重启前关闭失败：%v", err)
	}
	h.start(allowAll{})
	response := h.request(http.MethodGet, "/api/v1/tags", "", nil)
	body, _ := io.ReadAll(response.Body)
	if !strings.Contains(string(body), "e2e-tag") {
		t.Fatalf("重启后数据丢失：%s", body)
	}
}

func Test_E2E_cancellation_cleans_runtime(t *testing.T) {
	h := newHarness(t, nil)
	defer h.close()
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.app.Run(runCtx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("取消运行失败：%v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("取消后未在期限内完成清理")
	}
	h.app = nil
}

func createContent(t *testing.T, h *harness) (*http.Response, *http.Response) {
	t.Helper()
	typeResponse := h.request(http.MethodPost, "/api/v1/article-types", `{"name":"e2e-type","meun":1}`, nil)
	tagResponse := h.request(http.MethodPost, "/api/v1/tags", `{"name":"e2e-tag"}`, nil)
	return typeResponse, tagResponse
}

func createArticle(t *testing.T, h *harness, articleType, tag *http.Response, suffix, status string) *http.Response {
	t.Helper()
	body := fmt.Sprintf("{\"title\":\"e2e-%s\",\"slug\":\"e2e-%s\",\"digest\":\"digest\",\"content\":\"content\",\"article_type_id\":%d,\"tag_ids\":[%d],\"status\":\"%s\"}", suffix, suffix, locationID(t, articleType), locationID(t, tag), status)
	response := h.request(http.MethodPost, "/api/v1/articles", body, nil)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建 Article 失败：%d", response.StatusCode)
	}
	return response
}

func locationID(t *testing.T, response *http.Response) int64 {
	t.Helper()
	parts := strings.Split(strings.TrimSuffix(response.Header.Get("Location"), "/"), "/")
	id, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil {
		t.Fatalf("解析 Location 主键失败：%v", err)
	}
	return id
}
func openPool(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("打开 E2E SQL 连接失败：%v", err)
	}
	return pool
}

var _ content.Authorizer = allowAll{}
