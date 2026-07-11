//go:build integration

package bootstrap_test

import (
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
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/PengYuee/SCYG.Blog/backend/internal/bootstrap"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

func Test_Application_StartReadyShutdown_with_random_database(t *testing.T) {
	// Given
	adminDSN := os.Getenv("SCYG_POSTGRES_ADMIN_DSN")
	if adminDSN == "" {
		t.Fatal("缺少安全的 PostgreSQL 管理 DSN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	name, targetDSN := createTemporaryDatabase(t, ctx, adminDSN)
	defer dropTemporaryDatabase(t, adminDSN, name)
	applyMigrations(t, targetDSN)
	listener, listenErr := net.Listen("tcp", "127.0.0.1:0")
	if listenErr != nil {
		t.Fatalf("分配随机端口: %v", listenErr)
	}
	port := fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
	_ = listener.Close()
	for key, value := range map[string]string{"SCYG_DATABASE_DSN": targetDSN, "SCYG_HTTP_HOST": "127.0.0.1", "SCYG_HTTP_PORT": port, "SCYG_APP_ENV": "test", "SCYG_DOCS_ENABLED": "true"} {
		t.Setenv(key, value)
	}
	app, err := bootstrap.New(ctx, bootstrap.Options{LogWriter: io.Discard}, bootstrap.DefaultDependencies())
	if err != nil {
		t.Fatalf("构造应用: %v", err)
	}
	defer func() { _ = app.Shutdown(context.Background()) }()
	if err = app.Start(); err != nil {
		t.Fatalf("启动应用: %v", err)
	}
	client := &http.Client{Timeout: 3 * time.Second}

	// When / Then
	for path, status := range map[string]int{"/live": 200, "/ready": 200, "/docs": 200, "/api/v1/articles": 200} {
		response, requestErr := client.Get("http://" + app.Address().String() + path)
		if requestErr != nil {
			t.Fatalf("请求 %s: %v", path, requestErr)
		}
		_ = response.Body.Close()
		if response.StatusCode != status {
			t.Fatalf("%s status=%d", path, response.StatusCode)
		}
	}
	if err = app.Shutdown(ctx); err != nil {
		t.Fatalf("关闭应用: %v", err)
	}
}

func createTemporaryDatabase(t *testing.T, ctx context.Context, adminDSN string) (string, string) {
	t.Helper()
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		t.Fatalf("生成数据库名: %v", err)
	}
	name := "scyg_t11_" + hex.EncodeToString(bytes)
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Fatalf("打开管理连接: %v", err)
	}
	defer admin.Close()
	if _, err = admin.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatalf("创建临时数据库: %v", err)
	}
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("解析管理 DSN: %v", err)
	}
	parsed.Path = "/" + name
	return name, parsed.String()
}

func applyMigrations(t *testing.T, dsn string) {
	t.Helper()
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("打开迁移连接: %v", err)
	}
	runner, err := migrations.New(pool, "")
	if err != nil {
		t.Fatalf("构造迁移器: %v", err)
	}
	if err = runner.Up(); err != nil {
		t.Fatalf("执行迁移: %v", err)
	}
	if err = runner.Close(); err != nil {
		t.Fatalf("关闭迁移器: %v", err)
	}
}

func dropTemporaryDatabase(t *testing.T, adminDSN, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Errorf("打开清理连接: %v", err)
		return
	}
	defer admin.Close()
	_, _ = admin.ExecContext(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, name)
	if _, err = admin.ExecContext(ctx, fmt.Sprintf(`DROP DATABASE "%s"`, strings.ReplaceAll(name, `"`, `""`))); err != nil {
		t.Errorf("删除临时数据库: %v", err)
	}
}
