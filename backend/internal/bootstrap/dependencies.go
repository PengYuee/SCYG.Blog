// Package bootstrap 是 API 进程的手工依赖组合根。
package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/gin-gonic/gin"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/blobstorage"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/httpserver"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
)

// Database 是 bootstrap 所需的数据库生命周期与就绪探针。
type Database interface {
	Ping(context.Context) error
	Close() error
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
}

// Dependencies 集中测试可替换的同类构造接缝，生产使用 DefaultDependencies。
type Dependencies struct {
	// LoadConfig 解析并验证启动配置。
	LoadConfig func(config.Options) (config.Config, error)
	// NewLogger 构造结构化日志器。
	NewLogger func(observability.LoggerOptions) (*slog.Logger, error)
	// NewTelemetry 构造遥测生命周期。
	NewTelemetry func(config.Telemetry) (Telemetry, error)
	// NewDatabase 构造并连通数据库。
	NewDatabase func(context.Context, database.Options) (Database, error)
	// NewMigration 使用独立连接构造迁移检查器。
	NewMigration func(config.DSN) (Migration, error)
	// NewContent 构造内容模块。
	NewContent func(Database, module.Authorizer, module.CurrentAuthorProvider, *blobstorage.Filesystem, module.ArticleImagePolicy) (*module.Module, error)
	// NewCleanupWorker 构造受管图片清理 worker。
	NewCleanupWorker func(CleanupRunner, time.Duration, *slog.Logger) (CleanupWorker, error)
	// NewREST 构造路由挂载函数。
	NewREST func(*module.Module, *observability.Health, bool) (func(*gin.Engine) error, error)
	// NewHTTP 构造通用 HTTP 服务器。
	NewHTTP func(httpserver.Options) (HTTPServer, error)
}

// Options 是启动来源和生产可替换策略。
type Options struct {
	// ConfigFile 是可选 YAML 配置路径。
	ConfigFile string
	// LogWriter 接收结构化日志。
	LogWriter io.Writer
	// Authorizer 是测试可注入策略；生产 nil 即 DenyAll。
	Authorizer module.Authorizer
	// LifecycleObserver 接收 App 实际完成的关闭事实；生产可省略。
	LifecycleObserver LifecycleObserver
}
