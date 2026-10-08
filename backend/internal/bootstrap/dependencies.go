// Package bootstrap 是 API 进程的手工依赖组合根。
package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/image"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/blobstorage"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/httpserver"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
	rest "github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest"
)

// Database 是 bootstrap 所需的数据库生命周期、就绪探针与 GORM handle。
type Database interface {
	Ping(context.Context) error
	Close() error
	GORM() *gorm.DB
}

// Telemetry 是 bootstrap 持有的遥测生命周期最小接口。
type Telemetry interface{ Shutdown(context.Context) error }

// Migration 是启动时迁移状态检查所需的最小接口。
type Migration interface {
	Version() (uint, bool, error)
	Close() error
}

// HTTPServer 是应用运行所需的 HTTP 生命周期最小接口。
type HTTPServer interface {
	Start() (net.Listener, <-chan error, error)
	Shutdown(context.Context) error
	Close() error
}

// Dependencies 集中测试可替换的同类构造接缝，生产使用 DefaultDependencies。
type Dependencies struct {
	LoadConfig   func(config.Options) (config.Config, error)
	NewLogger    func(observability.LoggerOptions) (*slog.Logger, error)
	NewTelemetry func(config.Telemetry) (Telemetry, error)
	NewDatabase  func(context.Context, database.Options) (Database, error)
	NewMigration func(config.DSN) (Migration, error)

	NewArticle       func(Database, content.Authorizer, content.Clock) (*article.Service, error)
	NewTaxonomy      func(Database, content.Authorizer, content.Clock) (*taxonomy.Service, error)
	NewImage         func(Database, content.Authorizer, content.CurrentAuthorProvider, *blobstorage.Filesystem, image.Policy, content.Clock) (*image.Service, error)
	NewArticleImages func(Database, content.Authorizer, content.CurrentAuthorProvider, content.Clock, *article.Service, *image.Service) (*application.ArticleImages, error)
	// NewArticleResponses assembles REST article/category snapshots and write results.
	NewArticleResponses func(Database, *article.Service, *taxonomy.Service, *application.ArticleImages) (*application.ArticleResponses, error)
	NewImageCleanup     func(Database, *blobstorage.Filesystem, image.Policy, content.Clock) (CleanupRunner, error)
	NewCleanupWorker    func(CleanupRunner, time.Duration, *slog.Logger) (CleanupWorker, error)
	NewREST             func(rest.Options) (func(*gin.Engine) error, error)
	NewHTTP             func(httpserver.Options) (HTTPServer, error)
}

// Options 是启动来源和生产可替换策略。
type Options struct {
	// ConfigFile 是可选 YAML 配置路径。
	ConfigFile string
	// DisableConfigEnvironment 禁止 SCYG_* 环境变量覆盖配置。
	DisableConfigEnvironment bool
	// LogWriter 接收结构化日志。
	LogWriter io.Writer
	// Authorizer 是测试可注入策略；生产 nil 即 DenyAll。
	Authorizer content.Authorizer
	// LifecycleObserver 接收 App 实际完成的关闭事实；生产可省略。
	LifecycleObserver LifecycleObserver
}
