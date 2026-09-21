//go:build integration

package bootstrap_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/PengYuee/SCYG.Blog/backend/internal/bootstrap"
	qadatabase "github.com/PengYuee/SCYG.Blog/backend/internal/qa/database"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

func Test_Application_StartReadyShutdown_with_random_database(t *testing.T) {
	configPath := os.Getenv("QA_CONFIG")
	if configPath == "" {
		t.Fatal("QA_CONFIG is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	isolated, err := qadatabase.New(ctx, configPath, "bootstrap_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = isolated.Close(cleanupCtx)
	}()
	applyMigrations(t, isolated.DSN())
	listener, listenErr := net.Listen("tcp", "127.0.0.1:0")
	if listenErr != nil {
		t.Fatalf("分配随机端口: %v", listenErr)
	}
	port := fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
	_ = listener.Close()
	configFile := filepath.Join(t.TempDir(), "runtime.yaml")
	runtimeConfig := fmt.Sprintf("app:\n  env: test\nhttp:\n  host: 127.0.0.1\n  port: %s\ndatabase:\n  dsn: %s\ndocs:\n  enabled: true\n", port, isolated.DSN())
	if err = os.WriteFile(configFile, []byte(runtimeConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := bootstrap.New(ctx, bootstrap.Options{ConfigFile: configFile, DisableConfigEnvironment: true, LogWriter: io.Discard}, bootstrap.DefaultDependencies())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = app.Shutdown(context.Background()) }()
	if err = app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	for path, status := range map[string]int{"/live": 200, "/ready": 200, "/docs": 200, "/openapi.yaml": 200, "/docs/assets/scalar.js": 200, "/api/v1/articles": 200} {
		response, requestErr := client.Get("http://" + app.Address().String() + path)
		if requestErr != nil {
			t.Fatalf("请求 %s: %v", path, requestErr)
		}
		_ = response.Body.Close()
		if response.StatusCode != status {
			t.Fatalf("%s status=%d", path, response.StatusCode)
		}
	}
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
