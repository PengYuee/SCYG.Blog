package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
)

// App 持有已完成迁移校验的 API 生命周期资源。
type App struct {
	config       config.Config
	health       *observability.Health
	server       HTTPServer
	telemetry    Telemetry
	database     Database
	mutex        sync.Mutex
	listener     net.Listener
	serveErrors  <-chan error
	shutdownDone chan struct{}
	shutdownErr  error
	shuttingDown bool
	stopped      bool
}

func newApp(cfg config.Config, health *observability.Health, server HTTPServer, telemetry Telemetry, db Database) *App {
	return &App{config: cfg, health: health, server: server, telemetry: telemetry, database: db}
}

// Start 同步绑定监听地址；重复调用返回现有运行状态。
func (app *App) Start() error {
	app.mutex.Lock()
	defer app.mutex.Unlock()
	if app.stopped {
		return errors.New("应用已经停止")
	}
	if app.listener != nil {
		return nil
	}
	listener, serveErrors, err := app.server.Start()
	if err != nil {
		app.health.Withdraw()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), app.config.HTTP().ShutdownTimeout())
		defer cancel()
		app.shutdownErr = errors.Join(fmt.Errorf("绑定 HTTP: %w", err), app.telemetry.Shutdown(shutdownCtx), app.database.Close())
		app.stopped = true
		return app.shutdownErr
	}
	app.listener, app.serveErrors = listener, serveErrors
	return nil
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

// Shutdown 先撤回 readiness，再按 HTTP、遥测、数据库顺序关闭；并发调用共享一次结果。
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
	app.mutex.Unlock()
	err := errors.Join(app.server.Shutdown(ctx), app.telemetry.Shutdown(ctx), app.database.Close())
	app.mutex.Lock()
	app.shutdownErr = err
	app.shuttingDown = false
	app.stopped = true
	close(done)
	app.mutex.Unlock()
	return err
}
