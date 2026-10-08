package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
)

const startupMessage = "API 服务已就绪"

// startupServer 为启动日志测试返回真实随机端口监听器。
type startupServer struct {
	listener net.Listener
	startErr error
}

// Start 返回测试持有的监听器及可用服务错误通道。
func (server *startupServer) Start() (net.Listener, <-chan error, error) {
	if server.startErr != nil {
		return nil, nil, server.startErr
	}
	errors := make(chan error)
	return server.listener, errors, nil
}

// Shutdown 关闭测试监听器。
func (server *startupServer) Shutdown(context.Context) error {
	if server.listener == nil {
		return nil
	}
	return server.listener.Close()
}

func (server *startupServer) Close() error { return server.Shutdown(context.Background()) }

// startupTelemetry 提供无副作用的测试遥测资源。
type startupTelemetry struct{}

func (startupTelemetry) Shutdown(context.Context) error { return nil }

// startupWorker 提供无副作用的测试 worker 生命周期。
type startupWorker struct{}

func (startupWorker) Start(context.Context) error { return nil }
func (startupWorker) Stop(context.Context) error  { return nil }

// startupDatabase 提供无副作用的测试数据库资源。
type startupDatabase struct{}

func (startupDatabase) Ping(context.Context) error { return nil }
func (startupDatabase) Close() error               { return nil }

func (startupDatabase) GORM() *gorm.DB { return nil }

// startupFailureWorker 为启动失败测试注入可重试停止结果。
type startupFailureWorker struct {
	startErr error
	stopErrs []error
	stops    int
}

func (worker *startupFailureWorker) Start(context.Context) error { return worker.startErr }
func (worker *startupFailureWorker) Stop(context.Context) error {
	worker.stops++
	if worker.stops <= len(worker.stopErrs) {
		return worker.stopErrs[worker.stops-1]
	}
	return nil
}

// startupFailureDatabase 记录启动失败后的数据库关闭次数。
type startupFailureDatabase struct{ closes int }

func (*startupFailureDatabase) Ping(context.Context) error { return nil }
func (database *startupFailureDatabase) Close() error      { database.closes++; return nil }

func (*startupFailureDatabase) GORM() *gorm.DB { return nil }

// startupFailureTelemetry 记录启动失败后的遥测关闭次数。
type startupFailureTelemetry struct{ closes int }

func (telemetry *startupFailureTelemetry) Shutdown(context.Context) error {
	telemetry.closes++
	return nil
}

// startupFailureServer 提供 bind 与地址解析失败并保持 Shutdown 幂等。
type startupFailureServer struct {
	listener net.Listener
	startErr error
	closed   bool
}

func (server *startupFailureServer) Start() (net.Listener, <-chan error, error) {
	if server.startErr != nil {
		return nil, nil, server.startErr
	}
	return server.listener, make(chan error), nil
}

func (server *startupFailureServer) Shutdown(context.Context) error {
	if server.listener == nil || server.closed {
		return nil
	}
	server.closed = true
	return server.listener.Close()
}

func (server *startupFailureServer) Close() error { return server.Shutdown(context.Background()) }

// invalidAddressListener 保留真实监听器资源但返回不可解析地址。
type invalidAddressListener struct{ net.Listener }

func (invalidAddressListener) Addr() net.Addr { return stringAddr("地址无效") }

func Test_App_Start_logs_actual_service_URL_once_when_ready(t *testing.T) {
	// Given
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建测试监听器失败：%v", err)
	}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	health, err := observability.NewHealth(startupDatabase{}.Ping, func(context.Context) error { return nil })
	if err != nil {
		t.Fatalf("创建健康检查失败：%v", err)
	}
	app := newApp(context.Background(), configForStartupTest(t), logger, health, &startupServer{listener: listener}, startupWorker{}, startupTelemetry{}, startupDatabase{}, nil)
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })

	// When
	if err = app.Start(context.Background()); err != nil {
		t.Fatalf("首次启动失败：%v", err)
	}
	if err = app.Start(context.Background()); err != nil {
		t.Fatalf("重复启动失败：%v", err)
	}

	// Then
	tcpListener, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("startup listener address type = %T", listener.Addr())
	}
	port := tcpListener.Port
	output := logs.String()
	if strings.Count(output, startupMessage) != 1 {
		t.Fatalf("就绪日志次数不正确：%q", output)
	}
	for _, want := range []string{
		"service_url=http://127.0.0.1:" + portString(port),
		"docs_url=http://127.0.0.1:" + portString(port) + "/docs",
		"openapi_url=http://127.0.0.1:" + portString(port) + "/openapi.yaml",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("启动日志缺少 %q：%q", want, output)
		}
	}
}

func Test_App_Start_emits_no_readiness_log_when_bind_fails(t *testing.T) {
	// Given
	logs := &bytes.Buffer{}
	health, err := observability.NewHealth(startupDatabase{}.Ping, func(context.Context) error { return nil })
	if err != nil {
		t.Fatalf("创建健康检查失败：%v", err)
	}
	app := newApp(context.Background(), configForStartupTest(t), slog.New(slog.NewTextHandler(logs, nil)), health, &startupServer{startErr: errors.New("端口已占用")}, startupWorker{}, startupTelemetry{}, startupDatabase{}, nil)
	err = app.Start(context.Background())

	// Then
	if err == nil || strings.Contains(logs.String(), startupMessage) {
		t.Fatalf("启动错误=%v，日志=%q", err, logs.String())
	}
}

func Test_App_Start_failure_keeps_dependencies_until_worker_stop_is_confirmed(t *testing.T) {
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
			database := &startupFailureDatabase{}
			telemetry := &startupFailureTelemetry{}
			worker := &startupFailureWorker{startErr: test.workerErr, stopErrs: []error{context.DeadlineExceeded}}
			server := &startupFailureServer{startErr: test.serverErr}
			if test.invalidURL {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				server.listener = invalidAddressListener{Listener: listener}
			}
			health, err := observability.NewHealth(database.Ping, func(context.Context) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			app := newApp(context.Background(), configForStartupTest(t), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), health, server, worker, telemetry, database, nil)
			startErr := app.Start(context.Background())
			retryStartErr := app.Start(context.Background())
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

func Test_startupURL_normalizes_wildcard_and_IPv6_hosts(t *testing.T) {
	tests := []struct {
		name string
		addr net.Addr
		want string
	}{
		{name: "IPv4 通配地址", addr: stringAddr("0.0.0.0:8080"), want: "http://127.0.0.1:8080"},
		{name: "IPv6 通配地址", addr: stringAddr("[::]:8080"), want: "http://[::1]:8080"},
		{name: "IPv6 具体地址", addr: stringAddr("[2001:db8::1]:8080"), want: "http://[2001:db8::1]:8080"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			got, err := startupURL(test.addr, "")

			// Then
			if err != nil || got != test.want {
				t.Fatalf("URL=%q err=%v，期望=%q", got, err, test.want)
			}
		})
	}
}

func Test_startupURL_rejects_unparseable_listener_address(t *testing.T) {
	// When
	got, err := startupURL(stringAddr("地址无效"), "")

	// Then
	if err == nil || got != "" || !strings.Contains(err.Error(), "拆分监听地址") {
		t.Fatalf("URL=%q err=%v", got, err)
	}
}

// stringAddr 提供可控制的 net.Addr 文本表示。
type stringAddr string

func (addr stringAddr) Network() string { return "tcp" }
func (addr stringAddr) String() string  { return string(addr) }

// configForStartupTest 从最小 YAML 构造不可变配置。
func configForStartupTest(t *testing.T) config.Config {
	t.Helper()
	path := t.TempDir() + "/config.yaml"
	contents := "database:\n  dsn: postgres://postgres:postgres@localhost:5432/scyg?sslmode=disable\n" // #nosec G101 -- local test fixture exercises DSN parsing
	if err := writeStartupConfig(path, contents); err != nil {
		t.Fatalf("写入测试配置失败：%v", err)
	}
	cfg, err := config.Load(config.Options{File: path})
	if err != nil {
		t.Fatalf("加载测试配置失败：%v", err)
	}
	return cfg
}

// writeStartupConfig 隔离测试配置文件写入。
func writeStartupConfig(path string, contents string) error {
	return os.WriteFile(path, []byte(contents), 0o600)
}

// portString 将实际监听端口转换为 URL 端口文本。
func portString(port int) string { return strconv.Itoa(port) }
