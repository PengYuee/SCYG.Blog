package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/image"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/authentication"
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
		NewArticle: func(resource Database, authorizer content.Authorizer, clock content.Clock) (*article.Service, error) {
			return article.New(resource.GORM(), article.Dependencies{Clock: clock, Authorizer: authorizer})
		},
		NewTaxonomy: func(resource Database, authorizer content.Authorizer, clock content.Clock) (*taxonomy.Service, error) {
			return taxonomy.New(resource.GORM(), taxonomy.Dependencies{Clock: clock, Authorizer: authorizer})
		},
		NewImage: func(resource Database, authorizer content.Authorizer, currentAuthor content.CurrentAuthorProvider, filesystem *blobstorage.Filesystem, policy image.Policy, clock content.Clock) (*image.Service, error) {
			return image.New(resource.GORM(), image.Dependencies{Authorizer: authorizer, CurrentAuthor: currentAuthor, Blob: image.NewFilesystemBlob(filesystem, policy), Clock: clock, Policy: policy})
		},
		NewArticleImages: func(resource Database, authorizer content.Authorizer, currentAuthor content.CurrentAuthorProvider, clock content.Clock, articles *article.Service, images *image.Service) (*application.ArticleImages, error) {
			return application.NewArticleImages(application.Dependencies{DB: resource.GORM(), Clock: clock, Authorizer: authorizer, CurrentAuthor: currentAuthor, Articles: articles, Images: images})
		},
		NewImageCleanup: func(resource Database, filesystem *blobstorage.Filesystem, policy image.Policy, clock content.Clock) (CleanupRunner, error) {
			return image.NewDatabaseCleanup(resource.GORM(), image.NewFilesystemBlob(filesystem, policy), clock, policy)
		},
		NewCleanupWorker: NewArticleImageCleanupWorker,
		NewREST:          func(options rest.Options) (func(*gin.Engine) error, error) { return rest.New(options) },
		NewHTTP:          func(options httpserver.Options) (HTTPServer, error) { return httpserver.New(options) },
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
	cfg, err := dependencies.LoadConfig(config.Options{File: options.ConfigFile, DisableEnvironment: options.DisableConfigEnvironment})
	if err != nil {
		return nil, fmt.Errorf("加载配置: %w", err)
	}
	authConfig := cfg.Auth()
	tokenService, err := identityauth.NewTokenService([]byte(authConfig.JWTSecret()), authConfig.Issuer(), authConfig.AccessTokenTTL(), time.Now)
	if err != nil {
		return nil, fmt.Errorf("构造认证 token 服务: %w", err)
	}
	authorizer := options.Authorizer
	currentAuthor := content.CurrentAuthorProvider(authentication.ContextCurrentAuthor{})
	if authorizer == nil {
		authorizer = authentication.ContextAuthorizer{}
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
	fail := func(cleanupContext context.Context, root error) error {
		shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(cleanupContext), cfg.HTTP().ShutdownTimeout())
		defer cancel()
		return stack.Close(shutdownContext, root)
	}
	databaseConfig := cfg.Database()
	db, err := dependencies.NewDatabase(ctx, database.Options{Logger: logger, DSN: databaseConfig.DSN().Value(), ConnMaxLifetime: databaseConfig.ConnMaxLifetime(), MaxOpenConns: databaseConfig.MaxOpenConns(), MaxIdleConns: databaseConfig.MaxIdleConns()})
	if err != nil {
		return nil, fail(ctx, fmt.Errorf("连接数据库: %w", err))
	}
	if nilLike(db) {
		return nil, fail(ctx, errors.New("数据库构造器返回空结果"))
	}
	stack = append(stack, cleanupStep{name: "数据库", close: func(context.Context) error { return db.Close() }})
	migration, err := dependencies.NewMigration(databaseConfig.DSN())
	if err != nil {
		return nil, fail(ctx, fmt.Errorf("构造迁移检查: %w", err))
	}
	if nilLike(migration) {
		return nil, fail(ctx, errors.New("迁移构造器返回空结果"))
	}
	stack = append(stack, cleanupStep{name: "迁移检查", close: func(context.Context) error { return migration.Close() }})
	version, dirty, versionErr := migration.Version()
	closeErr := migration.Close()
	stack = stack[:len(stack)-1]
	if closeErr != nil {
		return nil, fail(ctx, fmt.Errorf("关闭迁移检查器: %w", closeErr))
	}
	if versionErr != nil || dirty || version != migrations.CurrentVersion {
		return nil, fail(ctx, errors.Join(fmt.Errorf("迁移状态无效: 当前版本=%d dirty=%t", version, dirty), versionErr))
	}
	var loginService *identityauth.LoginService
	if db.GORM() != nil {
		userRepository, repositoryErr := user.NewRepository(db.GORM())
		if repositoryErr != nil {
			return nil, fail(ctx, fmt.Errorf("构造用户仓储: %w", repositoryErr))
		}
		loginService, err = identityauth.NewLoginService(userRepository, tokenService)
		if err != nil {
			return nil, fail(ctx, fmt.Errorf("构造登录服务: %w", err))
		}
	}
	configuredAuthorID := cfg.ArticleImages().DevelopmentAuthorID()
	if cfg.App().Environment() == config.EnvironmentDevelopment && configuredAuthorID != "" {
		authorID, parseErr := content.NewAuthorID(configuredAuthorID)
		if parseErr != nil {
			return nil, fail(ctx, fmt.Errorf("构造开发作者身份: %w", parseErr))
		}
		currentAuthor = content.NewFixedCurrentAuthorProvider(authorID)
		if options.Authorizer == nil {
			authorizer = content.NewDevelopmentAuthorizer(authorID)
		}
	}
	imageConfig := cfg.ArticleImages()
	imagePolicy := image.NewPolicy(image.PolicyOptions{MaxFileBytes: imageConfig.MaxFileBytes(), MaxPixels: imageConfig.MaxPixels(), MaxDimension: imageConfig.MaxDimension(), PendingTTL: imageConfig.PendingTTL(), OrphanGrace: imageConfig.OrphanGrace()})
	storageDirectory, pathErr := filepath.Abs(imageConfig.Directory())
	if pathErr != nil {
		return nil, fail(ctx, fmt.Errorf("解析图片存储目录: %w", pathErr))
	}
	imageFilesystem, storageErr := blobstorage.New(storageDirectory)
	if storageErr != nil {
		return nil, fail(ctx, fmt.Errorf("构造图片存储: %w", storageErr))
	}
	stack = append(stack, cleanupStep{name: "图片存储", close: func(context.Context) error { return imageFilesystem.Close() }})
	clock := &systemClock{}
	articleService, err := dependencies.NewArticle(db, authorizer, clock)
	if err != nil {
		return nil, fail(ctx, fmt.Errorf("构造文章 feature: %w", err))
	}
	if articleService == nil {
		return nil, fail(ctx, errors.New("文章 feature 构造器返回空结果"))
	}
	taxonomyService, err := dependencies.NewTaxonomy(db, authorizer, clock)
	if err != nil {
		return nil, fail(ctx, fmt.Errorf("构造 taxonomy feature: %w", err))
	}
	if taxonomyService == nil {
		return nil, fail(ctx, errors.New("taxonomy feature 构造器返回空结果"))
	}
	imageService, err := dependencies.NewImage(db, authorizer, currentAuthor, imageFilesystem, imagePolicy, clock)
	if err != nil {
		return nil, fail(ctx, fmt.Errorf("构造图片 feature: %w", err))
	}
	if imageService == nil {
		return nil, fail(ctx, errors.New("图片 feature 构造器返回空结果"))
	}
	articleImages, err := dependencies.NewArticleImages(db, authorizer, currentAuthor, clock, articleService, imageService)
	if err != nil {
		return nil, fail(ctx, fmt.Errorf("构造文章图片协作: %w", err))
	}
	if articleImages == nil {
		return nil, fail(ctx, errors.New("文章图片协作构造器返回空结果"))
	}
	cleanupRunner, err := dependencies.NewImageCleanup(db, imageFilesystem, imagePolicy, clock)
	if err != nil {
		return nil, fail(ctx, fmt.Errorf("构造图片清理: %w", err))
	}
	if nilLike(cleanupRunner) {
		return nil, fail(ctx, errors.New("图片清理构造器返回空结果"))
	}
	worker, workerErr := dependencies.NewCleanupWorker(cleanupRunner, imageConfig.CleanupInterval(), logger)
	if workerErr != nil {
		return nil, fail(ctx, fmt.Errorf("构造图片清理 worker: %w", workerErr))
	}
	if nilLike(worker) {
		return nil, fail(ctx, errors.New("图片清理 worker 构造器返回空结果"))
	}
	health, err := observability.NewHealth(db.Ping, func(context.Context) error { return nil })
	if err != nil {
		return nil, fail(ctx, fmt.Errorf("构造健康检查: %w", err))
	}
	mount, err := dependencies.NewREST(rest.Options{ArticleQueries: articleService, ArticleCommands: articleImages, ArticleDeleter: articleService, Taxonomy: taxonomyService, Images: imageService, ImagePolicy: imagePolicy, Health: health, DocsEnabled: cfg.Docs().Enabled(), TokenVerifier: tokenService, Login: loginService})
	if err != nil {
		return nil, fail(ctx, fmt.Errorf("构造 REST: %w", err))
	}
	if nilLike(mount) {
		return nil, fail(ctx, errors.New("REST 构造器返回空结果"))
	}
	server, err := dependencies.NewHTTP(httpserver.Options{Logger: logger, Mount: mount, HTTP: cfg.HTTP(), ArticleImages: imageConfig})
	if err != nil {
		return nil, fail(ctx, fmt.Errorf("构造 HTTP: %w", err))
	}
	if nilLike(server) {
		return nil, fail(ctx, errors.New("HTTP 构造器返回空结果"))
	}
	return newApp(ctx, cfg, logger, health, server, worker, telemetry, &databaseWithStorage{Database: db, storage: imageFilesystem}, options.LifecycleObserver), nil
}

// databaseWithStorage 保持既有关闭顺序，并在数据库后关闭固定根句柄。
type databaseWithStorage struct {
	Database
	storage *blobstorage.Filesystem
}

func (resource *databaseWithStorage) Close() error {
	return errors.Join(resource.Database.Close(), resource.storage.Close())
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
