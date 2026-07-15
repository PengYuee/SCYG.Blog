package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
)

// cleanupStartFailureWorker 为启动失败测试注入可重试停止结果。
type cleanupStartFailureWorker struct {
	startErr error
	stopErrs []error
	stops    int
}

func (worker *cleanupStartFailureWorker) Start(context.Context) error { return worker.startErr }
func (worker *cleanupStartFailureWorker) Stop(context.Context) error {
	worker.stops++
	if worker.stops <= len(worker.stopErrs) {
		return worker.stopErrs[worker.stops-1]
	}
	return nil
}

// cleanupStartFailureDatabase 记录数据库关闭次数。
type cleanupStartFailureDatabase struct{ closes int }

func (*cleanupStartFailureDatabase) Ping(context.Context) error { return nil }
func (database *cleanupStartFailureDatabase) Close() error      { database.closes++; return nil }

// cleanupStartFailureTelemetry 记录遥测关闭次数。
type cleanupStartFailureTelemetry struct{ closes int }

func (telemetry *cleanupStartFailureTelemetry) Shutdown(context.Context) error {
	telemetry.closes++
	return nil
}

// cleanupStartFailureServer 提供 bind 与地址解析失败并保持 Shutdown 幂等。
type cleanupStartFailureServer struct {
	listener net.Listener
	startErr error
	closed   bool
}

func (server *cleanupStartFailureServer) Start() (net.Listener, <-chan error, error) {
	if server.startErr != nil {
		return nil, nil, server.startErr
	}
	return server.listener, make(chan error), nil
}

func (server *cleanupStartFailureServer) Shutdown(context.Context) error {
	if server.listener == nil || server.closed {
		return nil
	}
	server.closed = true
	return server.listener.Close()
}

// cleanupInvalidAddressListener 返回不可解析地址但保留真实监听资源。
type cleanupInvalidAddressListener struct{ net.Listener }

func (cleanupInvalidAddressListener) Addr() net.Addr { return cleanupStringAddr("地址无效") }

// cleanupStringAddr 提供可控监听地址文本。
type cleanupStringAddr string

func (addr cleanupStringAddr) Network() string { return "tcp" }
func (addr cleanupStringAddr) String() string  { return string(addr) }

func Test_CleanupWorker_start_failure_keeps_dependencies_until_stop_is_confirmed(t *testing.T) {
	tests := []struct {
		name       string
		workerErr  error
		serverErr  error
		invalidURL bool
	}{
		{name: "worker 启动失败", workerErr: errors.New("worker 启动失败")},
		{name: "HTTP bind 失败", serverErr: errors.New("端口已占用")},
		{name: "监听地址解析失败", invalidURL: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			database := &cleanupStartFailureDatabase{}
			telemetry := &cleanupStartFailureTelemetry{}
			worker := &cleanupStartFailureWorker{startErr: test.workerErr, stopErrs: []error{context.DeadlineExceeded}}
			server := &cleanupStartFailureServer{startErr: test.serverErr}
			if test.invalidURL {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				server.listener = cleanupInvalidAddressListener{Listener: listener}
			}
			health, err := observability.NewHealth(database.Ping, func(context.Context) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
			app := newApp(context.Background(), cleanupStartFailureConfig(t), logger, health, server, worker, telemetry, database, nil)

			// When
			startErr := app.Start()
			retryStartErr := app.Start()
			closesBeforeShutdown := database.closes + telemetry.closes
			shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			shutdownErr := app.Shutdown(shutdownCtx)

			// Then
			if startErr == nil || retryStartErr == nil || closesBeforeShutdown != 0 || shutdownErr != nil || database.closes != 1 || telemetry.closes != 1 {
				t.Fatalf("startErr=%v retryStartErr=%v closesBefore=%d shutdownErr=%v db=%d telemetry=%d", startErr, retryStartErr, closesBeforeShutdown, shutdownErr, database.closes, telemetry.closes)
			}
		})
	}
}

// cleanupStartFailureConfig 构造启动失败测试所需配置。
func cleanupStartFailureConfig(t *testing.T) config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := "database:\n  dsn: postgres://postgres:postgres@localhost:5432/scyg?sslmode=disable\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Options{File: path})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
