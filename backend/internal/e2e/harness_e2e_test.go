//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/PengYuee/SCYG.Blog/backend/internal/bootstrap"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	qaconfig "github.com/PengYuee/SCYG.Blog/backend/internal/qa/config"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

// allowAll 只编译进 e2e 测试二进制，生产构件不会包含该授权器。
type allowAll struct{}

// Authorize 允许真实 E2E 数据库执行写入。
func (allowAll) Authorize(context.Context, content.Action, content.Resource) error { return nil }

// harness 持有每个叙事独享的真实 PostgreSQL、应用与 HTTP 客户端。
type harness struct {
	t        *testing.T
	ctx      context.Context
	cancel   context.CancelFunc
	adminDSN string
	dsn      string
	name     string
	app      *bootstrap.App
	client   *http.Client
	baseURL  string
	observer bootstrap.LifecycleObserver
	// authorizer 在隔离请求间重建应用时保持授权语义。
	authorizer content.Authorizer
	// restartPerRequest 禁止同一 Gin Engine 处理相邻 CRUD 请求。
	restartPerRequest bool
}

// newHarness 创建随机数据库、执行真实迁移并启动真实 bootstrap HTTP 应用。
func newHarness(t *testing.T, authorizer content.Authorizer) *harness {
	return newHarnessWithObserver(t, authorizer, nil)
}

// newHarnessWithObserver 为信号叙事注入 App-owned 生命周期观察器。
func newHarnessWithObserver(t *testing.T, authorizer content.Authorizer, observer bootstrap.LifecycleObserver) *harness {
	t.Helper()
	qaConfig, err := qaconfig.LoadLocal()
	if err != nil {
		t.Fatalf("加载 E2E QA 配置失败：%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), qaConfig.CommandTimeout())
	adminDSN := qaConfig.AdminDSN().Value()
	name, dsn := createDatabase(t, ctx, qaConfig)
	h := &harness{t: t, ctx: ctx, cancel: cancel, adminDSN: adminDSN, dsn: dsn, name: name, client: &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}, observer: observer, authorizer: authorizer}
	h.migrateUp()
	h.start(authorizer)
	return h
}

// start 使用随机端口和真实默认依赖启动应用。
func (h *harness) start(authorizer content.Authorizer) {
	h.t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		h.t.Fatalf("分配 E2E 随机端口失败：%v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err = listener.Close(); err != nil {
		h.t.Fatalf("释放 E2E 预留端口失败：%v", err)
	}
	configFile := filepath.Join(h.t.TempDir(), "runtime.yaml")
	runtimeConfig := fmt.Sprintf("app:\n  env: test\nhttp:\n  host: 127.0.0.1\n  port: %d\ndatabase:\n  dsn: %s\ndocs:\n  enabled: true\n", port, h.dsn)
	if err = os.WriteFile(configFile, []byte(runtimeConfig), 0o600); err != nil {
		h.t.Fatalf("写入 E2E 运行配置失败：%v", err)
	}
	h.app, err = bootstrap.New(h.ctx, bootstrap.Options{ConfigFile: configFile, LogWriter: io.Discard, Authorizer: authorizer, LifecycleObserver: h.observer}, bootstrap.DefaultDependencies())
	if err != nil {
		h.t.Fatalf("构造 E2E 应用失败：%v", err)
	}
	if err = h.app.Start(); err != nil {
		h.t.Fatalf("启动 E2E 应用失败：%v", err)
	}
	h.baseURL = "http://" + h.app.Address().String()
}

// request 通过真实 TCP HTTP 边界发送请求并返回响应。
func (h *harness) request(method, path, body string, headers map[string]string) *http.Response {
	h.t.Helper()
	if h.app == nil {
		h.start(h.authorizer)
	}
	request, err := http.NewRequestWithContext(h.ctx, method, h.baseURL+path, bytes.NewBufferString(body))
	if err != nil {
		h.t.Fatalf("构造 E2E 请求失败：%v", err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	request.Close = true
	response, err := h.client.Do(request)
	if err != nil {
		h.t.Fatalf("执行 E2E 请求失败：%v", err)
	}
	payload, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		h.t.Fatalf("完整读取并关闭 E2E 响应失败：read=%v close=%v", readErr, closeErr)
	}
	response.Body = io.NopCloser(bytes.NewReader(payload))
	response.ContentLength = int64(len(payload))
	if h.restartPerRequest {
		if err := h.app.Shutdown(h.ctx); err != nil {
			h.t.Fatalf("关闭隔离 E2E 应用失败：%v", err)
		}
		h.app = nil
	}
	return response
}

// migrateUp 对独享数据库执行嵌入迁移。
func (h *harness) migrateUp() {
	h.migrate(func(runner *migrations.Runner) error { return runner.Up() })
}

// migrateDown 回滚独享数据库全部迁移。
func (h *harness) migrateDown() {
	h.migrate(func(runner *migrations.Runner) error { return runner.Down() })
}

func (h *harness) migrate(operation func(*migrations.Runner) error) {
	h.t.Helper()
	pool, err := sql.Open("pgx", h.dsn)
	if err != nil {
		h.t.Fatalf("打开 E2E 迁移连接失败：%v", err)
	}
	runner, err := migrations.New(pool, "")
	if err != nil {
		h.t.Fatalf("构造 E2E 迁移器失败：%v", err)
	}
	if err = operation(runner); err != nil {
		h.t.Fatalf("执行 E2E 迁移失败：%v", err)
	}
	if err = runner.Close(); err != nil {
		h.t.Fatalf("关闭 E2E 迁移器失败：%v", err)
	}
}

// close 有界关闭应用并删除随机数据库。
func (h *harness) close() {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if h.app != nil {
		if err := h.app.Shutdown(shutdownCtx); err != nil {
			h.t.Errorf("关闭 E2E 应用失败：%v", err)
		}
	}
	h.cancel()
	dropDatabase(h.t, h.adminDSN, h.name)
}

func createDatabase(t *testing.T, ctx context.Context, qaConfig qaconfig.Config) (string, string) {
	t.Helper()
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("生成 E2E 数据库名失败：%v", err)
	}
	name := qaConfig.DatabasePrefix() + hex.EncodeToString(random)
	adminDSN := qaConfig.AdminDSN().Value()
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Fatalf("打开 E2E 管理连接失败：%v", err)
	}
	defer admin.Close()
	if _, err = admin.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatalf("创建 E2E 数据库失败：%v", err)
	}
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("解析 E2E 管理 DSN 失败：%v", err)
	}
	parsed.Path = "/" + name
	return name, parsed.String()
}

func dropDatabase(t *testing.T, adminDSN, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Errorf("打开 E2E 清理连接失败：%v", err)
		return
	}
	defer admin.Close()
	_, _ = admin.ExecContext(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, name)
	if _, err = admin.ExecContext(ctx, fmt.Sprintf(`DROP DATABASE "%s"`, strings.ReplaceAll(name, `"`, `""`))); err != nil {
		t.Errorf("删除 E2E 数据库失败：%v", err)
	}
}
