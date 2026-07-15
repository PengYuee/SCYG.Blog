package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	contentpostgres "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/postgres"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/blobstorage"
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
		NewContent: func(resource Database, authorizer module.Authorizer, currentAuthor module.CurrentAuthorProvider, filesystem *blobstorage.Filesystem, imagePolicy module.ArticleImagePolicy) (*module.Module, error) {
			db, ok := resource.(*database.Database)
			if !ok {
				return nil, errors.New("数据库资源类型不正确")
			}
			return contentpostgres.New(contentpostgres.Dependencies{Database: db, Authorizer: authorizer, CurrentAuthor: currentAuthor, ImageFilesystem: filesystem, ImagePolicy: imagePolicy})
		},
		NewREST: func(content *module.Module, health *observability.Health, docs bool) (func(*gin.Engine) error, error) {
			return rest.New(rest.Options{Content: content, Health: health, DocsEnabled: docs})
		},
		NewHTTP: func(options httpserver.Options) (HTTPServer, error) { return httpserver.New(options) },
	}
}

// New 按严格顺序构造应用；任一步失败都按资源创建逆序关闭。
func New(ctx context.Context, options Options, dependencies Dependencies) (*App, error) {
	if err := validateDependencies(dependencies); err != nil {
		return nil, err
	}
	writer := options.LogWriter
	if writer == nil {
		writer = io.Writer(os.Stderr)
	}
	if nilLike(writer) {
		return nil, errors.New("日志输出为空")
	}
	cfg, err := dependencies.LoadConfig(config.Options{File: options.ConfigFile})
	if err != nil {
		return nil, fmt.Errorf("加载配置: %w", err)
	}
	logger, err := dependencies.NewLogger(observability.LoggerOptions{Writer: writer, Environment: string(cfg.App().Environment()), Level: string(cfg.App().LogLevel())})
	if err != nil {
		return nil, fmt.Errorf("构造日志器: %w", err)
	}
	if logger == nil {
		return nil, errors.New("日志构造器返回空结果")
	}
	telemetry, err := dependencies.NewTelemetry(cfg.Telemetry())
	if err != nil {
		return nil, fmt.Errorf("构造遥测: %w", err)
	}
	if nilLike(telemetry) {
		return nil, errors.New("遥测构造器返回空结果")
	}
	stack := cleanupStack{{name: "遥测", close: telemetry.Shutdown}}
	cleanupContext := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.WithoutCancel(ctx), cfg.HTTP().ShutdownTimeout())
	}
	fail := func(root error) error {
		cleanupCtx, cancel := cleanupContext()
		defer cancel()
		return stack.Close(cleanupCtx, root)
	}
	databaseConfig := cfg.Database()
	db, err := dependencies.NewDatabase(ctx, database.Options{Logger: logger, DSN: databaseConfig.DSN().Value(), ConnMaxLifetime: databaseConfig.ConnMaxLifetime(), MaxOpenConns: databaseConfig.MaxOpenConns(), MaxIdleConns: databaseConfig.MaxIdleConns()})
	if err != nil {
		return nil, fail(fmt.Errorf("连接数据库: %w", err))
	}
	if nilLike(db) {
		return nil, fail(errors.New("数据库构造器返回空结果"))
	}
	stack = append(stack, cleanupStep{name: "数据库", close: func(context.Context) error { return db.Close() }})
	migration, err := dependencies.NewMigration(databaseConfig.DSN())
	if err != nil {
		return nil, fail(fmt.Errorf("构造迁移检查: %w", err))
	}
	if nilLike(migration) {
		return nil, fail(errors.New("迁移构造器返回空结果"))
	}
	stack = append(stack, cleanupStep{name: "迁移检查器", close: func(context.Context) error { return migration.Close() }})
	version, dirty, versionErr := migration.Version()
	if versionErr != nil || dirty || version != migrations.CurrentVersion {
		return nil, fail(errors.Join(fmt.Errorf("迁移状态无效: 当前版本=%d dirty=%t", version, dirty), versionErr))
	}
	if closeErr := migration.Close(); closeErr != nil {
		return nil, fail(fmt.Errorf("关闭迁移检查器: %w", closeErr))
	}
	stack = stack[:len(stack)-1]
	authorizer := options.Authorizer
	var currentAuthor module.CurrentAuthorProvider
	configuredAuthorID := cfg.ArticleImages().DevelopmentAuthorID()
	if cfg.App().Environment() == config.EnvironmentDevelopment && configuredAuthorID != "" {
		authorID, parseErr := module.NewAuthorID(configuredAuthorID)
		if parseErr != nil {
			return nil, fail(fmt.Errorf("构造开发作者身份: %w", parseErr))
		}
		fixed := module.NewFixedCurrentAuthorProvider(authorID)
		currentAuthor = fixed
		if authorizer == nil {
			authorizer = module.NewDevelopmentAuthorizer(authorID)
		}
	}
	imageConfig := cfg.ArticleImages()
	imagePolicy := module.NewArticleImagePolicy(module.ArticleImagePolicyOptions{MaxFileBytes: imageConfig.MaxFileBytes(), MaxPixels: imageConfig.MaxPixels(), MaxDimension: imageConfig.MaxDimension(), PendingTTL: imageConfig.PendingTTL(), OrphanGrace: imageConfig.OrphanGrace()})
	storageDirectory, pathErr := filepath.Abs(cfg.ArticleImages().Directory())
	if pathErr != nil {
		return nil, fail(fmt.Errorf("解析图片存储目录: %w", pathErr))
	}
	imageFilesystem, storageErr := blobstorage.New(storageDirectory)
	if storageErr != nil {
		return nil, fail(fmt.Errorf("构造图片存储: %w", storageErr))
	}
	stack = append(stack, cleanupStep{name: "图片存储", close: func(context.Context) error { return imageFilesystem.Close() }})
	content, err := dependencies.NewContent(db, authorizer, currentAuthor, imageFilesystem, imagePolicy)
	if err != nil {
		return nil, fail(fmt.Errorf("构造内容模块: %w", err))
	}
	if content == nil {
		return nil, fail(errors.New("内容构造器返回空结果"))
	}
	health, err := observability.NewHealth(db.Ping, func(context.Context) error { return nil })
	if err != nil {
		return nil, fail(fmt.Errorf("构造健康检查: %w", err))
	}
	mount, err := dependencies.NewREST(content, health, cfg.Docs().Enabled())
	if err != nil {
		return nil, fail(fmt.Errorf("构造 REST: %w", err))
	}
	if nilLike(mount) {
		return nil, fail(errors.New("REST 构造器返回空结果"))
	}
	server, err := dependencies.NewHTTP(httpserver.Options{Logger: logger, Mount: mount, HTTP: cfg.HTTP(), ArticleImages: cfg.ArticleImages()})
	if err != nil {
		return nil, fail(fmt.Errorf("构造 HTTP: %w", err))
	}
	if nilLike(server) {
		return nil, fail(errors.New("HTTP 构造器返回空结果"))
	}
	return newApp(cfg, logger, health, server, telemetry, &databaseWithStorage{Database: db, storage: imageFilesystem}, options.LifecycleObserver), nil
}

// databaseWithStorage 保持既有关闭顺序，并在数据库后关闭固定根句柄。
type databaseWithStorage struct {
	Database
	storage *blobstorage.Filesystem
}

func (resource *databaseWithStorage) Close() error {
	return errors.Join(resource.Database.Close(), resource.storage.Close())
}
