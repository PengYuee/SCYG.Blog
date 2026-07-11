package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gin-gonic/gin"

	_ "github.com/jackc/pgx/v5/stdlib"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	contentpostgres "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/postgres"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/httpserver"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
	rest "github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

// DefaultDependencies 返回不含全局状态的生产构造函数集合。
func DefaultDependencies() Dependencies {
	return Dependencies{
		LoadConfig:   config.Load,
		NewLogger:    observability.NewLogger,
		NewTelemetry: func(config.Telemetry) (Telemetry, error) { return observability.NewTelemetry(), nil },
		NewDatabase: func(ctx context.Context, options database.Options) (Database, error) {
			return database.New(ctx, options)
		},
		NewMigration: func(dsn config.DSN) (Migration, error) {
			pool, err := sql.Open("pgx", dsn.Value())
			if err != nil {
				return nil, fmt.Errorf("打开迁移数据库: %w", err)
			}
			runner, err := migrations.New(pool, "")
			if err != nil {
				return nil, errors.Join(fmt.Errorf("构造迁移检查器: %w", err), pool.Close())
			}
			return runner, nil
		},
		NewContent: func(resource Database, authorizer module.Authorizer) (*module.Module, error) {
			db, ok := resource.(*database.Database)
			if !ok {
				return nil, errors.New("数据库资源类型不正确")
			}
			return contentpostgres.New(contentpostgres.Dependencies{Database: db, Authorizer: authorizer})
		},
		NewREST: func(content *module.Module, health *observability.Health, docs bool) (func(*gin.Engine) error, error) {
			return rest.New(rest.Options{Content: content, Health: health, DocsEnabled: docs})
		},
		NewHTTP: func(options httpserver.Options) (HTTPServer, error) { return httpserver.New(options) },
	}
}

// New 按严格顺序构造应用；任一步失败都反向关闭已创建资源。
func New(ctx context.Context, options Options, dependencies Dependencies) (_ *App, err error) {
	if dependencies.LoadConfig == nil {
		dependencies = DefaultDependencies()
	}
	writer := options.LogWriter
	if writer == nil {
		writer = io.Writer(os.Stderr)
	}
	cfg, err := dependencies.LoadConfig(config.Options{File: options.ConfigFile})
	if err != nil {
		return nil, fmt.Errorf("加载配置: %w", err)
	}
	logger, err := dependencies.NewLogger(observability.LoggerOptions{Writer: writer, Environment: string(cfg.App().Environment()), Level: string(cfg.App().LogLevel())})
	if err != nil {
		return nil, fmt.Errorf("构造日志器: %w", err)
	}
	telemetry, err := dependencies.NewTelemetry(cfg.Telemetry())
	if err != nil {
		return nil, fmt.Errorf("构造遥测: %w", err)
	}
	cleanup := func(root error, db Database) error {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.HTTP().ShutdownTimeout())
		defer cancel()
		if db == nil {
			return errors.Join(root, telemetry.Shutdown(shutdownCtx))
		}
		return errors.Join(root, telemetry.Shutdown(shutdownCtx), db.Close())
	}
	databaseConfig := cfg.Database()
	db, err := dependencies.NewDatabase(ctx, database.Options{Logger: logger, DSN: databaseConfig.DSN().Value(), ConnMaxLifetime: databaseConfig.ConnMaxLifetime(), MaxOpenConns: databaseConfig.MaxOpenConns(), MaxIdleConns: databaseConfig.MaxIdleConns()})
	if err != nil {
		return nil, cleanup(fmt.Errorf("连接数据库: %w", err), nil)
	}
	migration, err := dependencies.NewMigration(databaseConfig.DSN())
	if err != nil {
		return nil, cleanup(fmt.Errorf("构造迁移检查: %w", err), db)
	}
	version, dirty, versionErr := migration.Version()
	migrationErr := migration.Close()
	if versionErr != nil || dirty || version != migrations.CurrentVersion {
		stateErr := fmt.Errorf("迁移状态无效: 当前版本=%d dirty=%t", version, dirty)
		return nil, cleanup(errors.Join(stateErr, versionErr, migrationErr), db)
	}
	if migrationErr != nil {
		return nil, cleanup(fmt.Errorf("关闭迁移检查器: %w", migrationErr), db)
	}
	content, err := dependencies.NewContent(db, options.Authorizer)
	if err != nil {
		return nil, cleanup(fmt.Errorf("构造内容模块: %w", err), db)
	}
	health, err := observability.NewHealth(db.Ping, func(context.Context) error { return nil })
	if err != nil {
		return nil, cleanup(fmt.Errorf("构造健康检查: %w", err), db)
	}
	mount, err := dependencies.NewREST(content, health, cfg.Docs().Enabled())
	if err != nil {
		return nil, cleanup(fmt.Errorf("构造 REST: %w", err), db)
	}
	server, err := dependencies.NewHTTP(httpserver.Options{Logger: logger, Mount: mount, HTTP: cfg.HTTP()})
	if err != nil {
		return nil, cleanup(fmt.Errorf("构造 HTTP: %w", err), db)
	}
	health.Activate()
	return newApp(cfg, health, server, telemetry, db), nil
}
