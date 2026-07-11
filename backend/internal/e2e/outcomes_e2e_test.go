//go:build e2e

package e2e_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

// databaseSnapshot 固定写拒绝和并发失败前后的持久化可观察状态。
type databaseSnapshot struct {
	ArticleTypes int
	Tags         int
	Articles     int
	TagArticles  int
}

// entitySnapshot 固定实体版本和内容，证明失败请求没有产生写入。
type entitySnapshot struct {
	Version uint64
	Content string
}

// snapshotDatabase 查询所有目标表行数，供前后 DeepEqual 比较。
func snapshotDatabase(t *testing.T, ctx context.Context, dsn string) databaseSnapshot {
	t.Helper()
	pool := openPool(t, dsn)
	defer pool.Close()
	result := databaseSnapshot{}
	for table, target := range map[string]*int{`"ArticleType"`: &result.ArticleTypes, `"Tag"`: &result.Tags, `"Article"`: &result.Articles, `"TagArticle"`: &result.TagArticles} {
		if err := pool.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(target); err != nil {
			t.Fatalf("读取数据库快照失败：%v", err)
		}
	}
	return result
}

// snapshotTag 查询 Tag 的版本和名称，作为 stale 写入前后快照。
func snapshotTag(t *testing.T, ctx context.Context, dsn string, id int64) entitySnapshot {
	t.Helper()
	pool := openPool(t, dsn)
	defer pool.Close()
	result := entitySnapshot{}
	if err := pool.QueryRowContext(ctx, `SELECT "Version", "Name" FROM "Tag" WHERE "Id"=$1`, id).Scan(&result.Version, &result.Content); err != nil {
		t.Fatalf("读取 Tag 快照失败：%v", err)
	}
	return result
}

// waitHTTPStatus 在 context 截止前轮询真实 readiness，不产生无限等待。
func waitHTTPStatus(ctx context.Context, client *http.Client, endpoint string, expected int) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr == nil {
				_ = response.Body.Close()
				if response.StatusCode == expected {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待 HTTP %d 超时：%w", expected, ctx.Err())
		case <-ticker.C:
		}
	}
}

// setDatabaseConnectionsAllowed 控制测试数据库接受新连接，并终止旧连接制造可恢复故障。
func setDatabaseConnectionsAllowed(t *testing.T, h *harness, allowed bool) {
	t.Helper()
	admin := openPool(t, h.adminDSN)
	defer admin.Close()
	limit := 0
	if allowed {
		limit = -1
	}
	if _, err := admin.ExecContext(h.ctx, fmt.Sprintf(`ALTER DATABASE "%s" CONNECTION LIMIT %d`, strings.ReplaceAll(h.name, `"`, `""`), limit)); err != nil {
		t.Fatalf("切换数据库连接状态失败：%v", err)
	}
	if !allowed {
		if _, err := admin.ExecContext(h.ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1`, h.name); err != nil {
			t.Fatalf("终止测试数据库连接失败：%v", err)
		}
	}
}

// localOnlyTransport 仅允许访问测试 API 主机，记录并拒绝所有外部请求。
type localOnlyTransport struct {
	allowedHost string
	external    []string
	base        http.RoundTripper
}

func (transport *localOnlyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host != transport.allowedHost {
		transport.external = append(transport.external, request.URL.String())
		return nil, errors.New("E2E 离线 transport 拒绝外部请求")
	}
	return transport.base.RoundTrip(request)
}

// assertLocalReferences 获取 HTML 中声明的本地 JS/spec，并证明外部 URL 被 transport 拒绝。
func assertLocalReferences(t *testing.T, h *harness, html string) {
	t.Helper()
	base, err := url.Parse(h.baseURL)
	if err != nil {
		t.Fatalf("解析测试 URL 失败：%v", err)
	}
	transport := &localOnlyTransport{allowedHost: base.Host, base: http.DefaultTransport}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	for _, path := range []string{"/scalar.js", "/openapi.yaml"} {
		response, requestErr := client.Get(h.baseURL + path)
		if requestErr != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("本地文档资源不可用：%s %v", path, requestErr)
		}
		_ = response.Body.Close()
		if !strings.Contains(html, path) {
			t.Fatalf("文档 HTML 未引用本地资源：%s", path)
		}
	}
	_, externalErr := client.Get("https://cdn.example.invalid/scalar.js")
	if externalErr == nil || !reflect.DeepEqual(transport.external, []string{"https://cdn.example.invalid/scalar.js"}) {
		t.Fatal("外部文档请求未被记录并拒绝")
	}
}

var _ http.RoundTripper = (*localOnlyTransport)(nil)
var _ = sql.ErrNoRows
var _ = net.ErrClosed
