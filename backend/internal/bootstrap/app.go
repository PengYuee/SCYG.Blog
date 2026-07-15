package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
)

// App 持有已完成迁移校验、尚未开放 readiness 的 API 生命周期资源。
type App struct {
	config config.Config
	// logger 复用组合根创建的结构化日志器。
	logger *slog.Logger
	health *observability.Health
	server HTTPServer
	// worker 在 HTTP ready 前启动，并在数据库关闭前停止。
	worker       CleanupWorker
	telemetry    Telemetry
	database     Database
	observer     LifecycleObserver
	mutex        sync.Mutex
	listener     net.Listener
	serveErrors  <-chan error
	shutdownDone chan struct{}
	shutdownErr  error
	shuttingDown bool
	stopped      bool
	// startFailed 表示启动清理尚未确认 worker 退出，只允许继续 Shutdown。
	startFailed bool
	// httpCleanup 表示 HTTP 已进入启动流程，关闭时需要执行 drain。
	httpCleanup bool
	// lifetime 为 worker 提供不受构造调用短期取消影响的应用上下文。
	lifetime context.Context
}

// newApp 汇集已经成功构造的应用生命周期资源。
func newApp(lifetime context.Context, cfg config.Config, logger *slog.Logger, health *observability.Health, server HTTPServer, worker CleanupWorker, telemetry Telemetry, db Database, observer LifecycleObserver) *App {
	return &App{config: cfg, logger: logger, health: health, server: server, worker: worker, telemetry: telemetry, database: db, observer: lifecycleObserverOrDefault(observer), lifetime: context.WithoutCancel(lifetime)}
}

// Start 同步绑定监听地址，并仅在启动结果完整有效后开放 readiness。
func (app *App) Start() error {
	app.mutex.Lock()
	defer app.mutex.Unlock()
	if app.stopped {
		return errors.New("应用已经停止")
	}
	if app.startFailed {
		return errors.New("应用启动失败，等待完成关闭")
	}
	if app.listener != nil {
		return nil
	}
	if err := app.worker.Start(app.lifetime); err != nil {
		return app.cleanupStartFailure(fmt.Errorf("启动图片清理 worker: %w", err), false)
	}
	app.httpCleanup = true
	listener, serveErrors, err := app.server.Start()
	if err == nil && nilLike(listener) {
		err = errors.New("HTTP 启动返回空监听器")
	}
	if err == nil && serveErrors == nil {
		err = errors.New("HTTP 启动返回空错误通道")
	}
	if err != nil {
		return app.cleanupStartFailure(fmt.Errorf("绑定 HTTP: %w", err), true)
	}
	if _, _, addressErr := net.SplitHostPort(listener.Addr().String()); addressErr != nil {
		return app.cleanupStartFailure(fmt.Errorf("解析 HTTP 监听地址: %w", addressErr), true)
	}
	app.listener, app.serveErrors = listener, serveErrors
	app.health.Activate()
	return nil
}

// cleanupStartFailure 统一回收启动失败资源；worker 未退出时保留其数据库依赖并允许 Shutdown 重试。
func (app *App) cleanupStartFailure(root error, cleanupHTTP bool) error {
	app.health.Withdraw()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), app.config.HTTP().ShutdownTimeout())
	defer cancel()
	var httpErr error
	if cleanupHTTP {
		httpErr = app.server.Shutdown(shutdownCtx)
	}
	workerErr := app.worker.Stop(shutdownCtx)
	app.shutdownErr = errors.Join(root, httpErr, workerErr)
	if workerErr != nil {
		app.startFailed = true
		app.httpCleanup = cleanupHTTP
		return app.shutdownErr
	}
	databaseErr := app.database.Close()
	telemetryErr := app.telemetry.Shutdown(shutdownCtx)
	app.shutdownErr = errors.Join(app.shutdownErr, databaseErr, telemetryErr)
	app.stopped = true
	return app.shutdownErr
}

// Address 返回 Start 成功后绑定的监听地址。
func (app *App) Address() net.Addr {
	app.mutex.Lock()
	defer app.mutex.Unlock()
	if app.listener == nil {
		return nil
	}
	return app.listener.Addr()
}

// Run 启动服务并等待信号上下文取消或服务器错误，随后执行有界关闭。
// 未来共享命运协议可在此处用 errgroup 并列运行；当前不创建未使用的 Runner 或协议抽象。
func (app *App) Run(ctx context.Context) error {
	if err := app.Start(); err != nil {
		return err
	}
	app.mutex.Lock()
	serveErrors := app.serveErrors
	app.mutex.Unlock()
	var root error
	select {
	case <-ctx.Done():
	case root = <-serveErrors:
		if root != nil {
			root = fmt.Errorf("HTTP 服务失败: %w", root)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), app.config.HTTP().ShutdownTimeout())
	defer cancel()
	return errors.Join(root, app.Shutdown(shutdownCtx))
}

// Shutdown 第一步撤回 readiness，再严格按 HTTP、worker、数据库、遥测顺序关闭；并发调用共享一次结果。
func (app *App) Shutdown(ctx context.Context) error {
	app.mutex.Lock()
	if app.stopped {
		err := app.shutdownErr
		app.mutex.Unlock()
		return err
	}
	if app.shuttingDown {
		done := app.shutdownDone
		app.mutex.Unlock()
		select {
		case <-done:
			app.mutex.Lock()
			err := app.shutdownErr
			app.mutex.Unlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	app.shuttingDown = true
	app.shutdownDone = make(chan struct{})
	done := app.shutdownDone
	app.health.Withdraw()
	app.observer.ReadinessWithdrawn()
	app.mutex.Unlock()
	// HTTP drain 后必须确认 worker 退出，才能关闭其依赖的数据库与遥测。
	var httpErr error
	if app.httpCleanup {
		httpErr = app.server.Shutdown(ctx)
	}
	if httpErr == nil {
		app.observer.HTTPClosed()
	}
	workerErr := app.worker.Stop(ctx)
	if workerErr != nil {
		// worker 未确认退出时保持数据库与遥测存活，允许后续 Shutdown 使用新期限继续等待。
		err := errors.Join(httpErr, workerErr)
		app.mutex.Lock()
		app.shutdownErr = err
		app.shuttingDown = false
		close(done)
		app.mutex.Unlock()
		return err
	}
	app.observer.WorkerStopped()
	databaseErr := app.database.Close()
	if databaseErr == nil {
		app.observer.DatabaseClosed()
	}
	telemetryErr := app.telemetry.Shutdown(ctx)
	if telemetryErr == nil {
		app.observer.TelemetryClosed()
	}
	err := errors.Join(httpErr, workerErr, databaseErr, telemetryErr)
	app.mutex.Lock()
	app.shutdownErr = err
	app.shuttingDown = false
	app.stopped = true
	app.startFailed = false
	close(done)
	app.mutex.Unlock()
	return err
}
