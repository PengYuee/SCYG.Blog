package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// Options controls the startup configuration source.
type Options struct {
	// File 是显式 YAML 路径；空路径仅使用内置默认值和允许的环境覆盖。
	File string
	// DisableEnvironment 禁止环境变量覆盖，供本地迁移等纯文件入口使用。
	DisableEnvironment bool
}

type rawConfig struct {
	App           rawApp           `mapstructure:"app"`
	Telemetry     rawTelemetry     `mapstructure:"telemetry"`
	Database      rawDatabase      `mapstructure:"database"`
	HTTP          rawHTTP          `mapstructure:"http"`
	Docs          rawDocs          `mapstructure:"docs"`
	ArticleImages rawArticleImages `mapstructure:"article_images"`
}

type rawApp struct {
	Environment string `mapstructure:"env"`
	LogLevel    string `mapstructure:"log_level"`
}

type rawHTTP struct {
	Host               string        `mapstructure:"host"`
	TrustedProxies     []string      `mapstructure:"trusted_proxies"`
	CORSAllowedOrigins []string      `mapstructure:"cors_allowed_origins"`
	Port               int           `mapstructure:"port"`
	ReadHeaderTimeout  time.Duration `mapstructure:"read_header_timeout"`
	ReadTimeout        time.Duration `mapstructure:"read_timeout"`
	WriteTimeout       time.Duration `mapstructure:"write_timeout"`
	IdleTimeout        time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout    time.Duration `mapstructure:"shutdown_timeout"`
}

type rawArticleImages struct {
	Directory           string        `mapstructure:"directory"`
	PendingTTL          time.Duration `mapstructure:"pending_ttl"`
	OrphanGrace         time.Duration `mapstructure:"orphan_grace"`
	CleanupInterval     time.Duration `mapstructure:"cleanup_interval"`
	UploadRequestBytes  int64         `mapstructure:"upload_request_bytes"`
	MaxFileBytes        int64         `mapstructure:"max_file_bytes"`
	MaxPixels           int64         `mapstructure:"max_pixels"`
	MaxDimension        int           `mapstructure:"max_dimension"`
	DevelopmentAuthorID string        `mapstructure:"development_author_id"`
}

type rawDatabase struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

type (
	rawDocs struct {
		Enabled bool `mapstructure:"enabled"`
	}
	rawTelemetry struct {
		OTLPEndpoint string `mapstructure:"otlp_endpoint"`
	}
)

// Load constructs one local Viper instance, parses all sources, and returns a validated value.
func Load(options Options) (Config, error) {
	instance := viper.New()
	setDefaults(instance)
	instance.SetEnvPrefix("SCYG")
	instance.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	keys := [...]string{
		"app.env", "app.log_level", "http.host", "http.port", "http.read_header_timeout", "http.read_timeout",
		"http.write_timeout", "http.idle_timeout", "http.shutdown_timeout", "http.trusted_proxies", "http.cors_allowed_origins",
		"database.dsn", "database.max_open_conns", "database.max_idle_conns", "database.conn_max_lifetime", "docs.enabled", "telemetry.otlp_endpoint",
		"article_images.directory", "article_images.pending_ttl", "article_images.orphan_grace", "article_images.cleanup_interval",
		"article_images.upload_request_bytes", "article_images.max_file_bytes", "article_images.max_pixels", "article_images.max_dimension", "article_images.development_author_id",
	}
	if !options.DisableEnvironment {
		for _, key := range keys {
			if err := instance.BindEnv(key); err != nil {
				return Config{}, fmt.Errorf("绑定环境配置 %s 失败：%w", key, err)
			}
		}
	}
	if options.File != "" {
		instance.SetConfigFile(options.File)
		instance.SetConfigType("yaml")
		if err := instance.ReadInConfig(); err != nil {
			return Config{}, &FileError{Path: options.File, Err: err}
		}
	}
	// QA 段由独立 loader 解析；运行时副本在解码前移除该段，避免管理 DSN 进入 Config。
	settings := instance.AllSettings()
	delete(settings, "qa")
	runtime := viper.New()
	if err := runtime.MergeConfigMap(settings); err != nil {
		return Config{}, fmt.Errorf("合并运行时配置失败：%w", err)
	}
	var raw rawConfig
	hook := mapstructure.ComposeDecodeHookFunc(mapstructure.StringToTimeDurationHookFunc(), mapstructure.StringToSliceHookFunc(","))
	if err := runtime.UnmarshalExact(&raw, viper.DecodeHook(hook)); err != nil {
		return Config{}, fmt.Errorf("解析配置失败：%w", err)
	}
	return validate(raw)
}

func setDefaults(instance *viper.Viper) {
	defaults := map[string]any{
		"app.env": "development", "app.log_level": "info", "http.host": "0.0.0.0", "http.port": 8080,
		"http.read_header_timeout": "5s", "http.read_timeout": "15s", "http.write_timeout": "15s", "http.idle_timeout": "60s", "http.shutdown_timeout": "10s",
		"http.trusted_proxies": []string{}, "http.cors_allowed_origins": []string{"http://localhost:5173"},
		"database.dsn": "postgres://postgres:" + "postgres@localhost:5432/scyg?sslmode=disable", "database.max_open_conns": 25, "database.max_idle_conns": 5,
		"database.conn_max_lifetime": "30m", "docs.enabled": true, "telemetry.otlp_endpoint": "",
		"article_images.directory": "data/article-images", "article_images.pending_ttl": "24h", "article_images.orphan_grace": "24h", "article_images.cleanup_interval": "1h",
		"article_images.upload_request_bytes": int64(6 << 20), "article_images.max_file_bytes": int64(5 << 20), "article_images.max_pixels": int64(25_000_000),
		"article_images.max_dimension": 8192, "article_images.development_author_id": "",
	}
	for key, value := range defaults {
		instance.SetDefault(key, value)
	}
}
