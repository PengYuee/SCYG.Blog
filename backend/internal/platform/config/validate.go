package config

import (
	"net"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func validate(raw rawConfig) (Config, error) {
	environment := Environment(raw.App.Environment)
	if environment != EnvironmentDevelopment && environment != EnvironmentTest && environment != EnvironmentProduction {
		return Config{}, invalid("app.env", "必须为 development、test 或 production")
	}
	level := LogLevel(raw.App.LogLevel)
	if level != LogLevelDebug && level != LogLevelInfo && level != LogLevelWarn && level != LogLevelError {
		return Config{}, invalid("app.log_level", "必须为 debug、info、warn 或 error")
	}
	if err := validateHTTP(raw.HTTP); err != nil {
		return Config{}, err
	}
	if err := validateDatabase(raw.Database); err != nil {
		return Config{}, err
	}
	if err := validateTelemetry(raw.Telemetry); err != nil {
		return Config{}, err
	}
	articleImages, err := validateArticleImages(raw.ArticleImages, environment)
	if err != nil {
		return Config{}, err
	}
	return Config{app: App{environment: environment, logLevel: level}, http: HTTP{host: raw.HTTP.Host, port: raw.HTTP.Port, readHeaderTimeout: raw.HTTP.ReadHeaderTimeout, readTimeout: raw.HTTP.ReadTimeout, writeTimeout: raw.HTTP.WriteTimeout, idleTimeout: raw.HTTP.IdleTimeout, shutdownTimeout: raw.HTTP.ShutdownTimeout, trustedProxies: append([]string(nil), raw.HTTP.TrustedProxies...), corsAllowedOrigins: append([]string(nil), raw.HTTP.CORSAllowedOrigins...)}, database: Database{dsn: DSN{value: raw.Database.DSN}, maxOpenConns: raw.Database.MaxOpenConns, maxIdleConns: raw.Database.MaxIdleConns, connMaxLifetime: raw.Database.ConnMaxLifetime}, docs: Docs{enabled: raw.Docs.Enabled}, telemetry: Telemetry{otlpEndpoint: raw.Telemetry.OTLPEndpoint}, articleImages: articleImages}, nil
}

func validateHTTP(raw rawHTTP) error {
	if strings.TrimSpace(raw.Host) == "" {
		return invalid("http.host", "不能为空")
	}
	if raw.Port < 1 || raw.Port > 65535 {
		return invalid("http.port", "必须介于 1 和 65535 之间")
	}
	durations := []struct {
		field string
		value time.Duration
	}{{"http.read_header_timeout", raw.ReadHeaderTimeout}, {"http.read_timeout", raw.ReadTimeout}, {"http.write_timeout", raw.WriteTimeout}, {"http.idle_timeout", raw.IdleTimeout}, {"http.shutdown_timeout", raw.ShutdownTimeout}}
	for _, duration := range durations {
		if duration.value <= 0 {
			return invalid(duration.field, "必须大于零")
		}
	}
	if err := validateProxies(raw.TrustedProxies); err != nil {
		return err
	}
	return validateOrigins(raw.CORSAllowedOrigins)
}

func validateProxies(proxies []string) error {
	for _, proxy := range proxies {
		if net.ParseIP(proxy) == nil {
			if _, _, err := net.ParseCIDR(proxy); err != nil {
				return invalid("http.trusted_proxies", "只能包含 IP 地址或 CIDR")
			}
		}
	}
	return nil
}

func validateOrigins(origins []string) error {
	for _, origin := range origins {
		parsed, err := url.ParseRequestURI(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return invalid("http.cors_allowed_origins", "只能包含绝对 HTTP 来源地址")
		}
	}
	return nil
}

func validateDatabase(raw rawDatabase) error {
	if strings.Contains(raw.DSN, "请填写密码") {
		return invalid("database.dsn", "请先填写数据库密码，不能使用占位值")
	}
	parsed, err := url.Parse(raw.DSN)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
		return invalid("database.dsn", "必须是含主机和数据库名的 PostgreSQL DSN")
	}
	if raw.MaxOpenConns < 1 {
		return invalid("database.max_open_conns", "必须大于零")
	}
	if raw.MaxIdleConns < 0 || raw.MaxIdleConns > raw.MaxOpenConns {
		return invalid("database.max_idle_conns", "必须介于零和 max_open_conns 之间")
	}
	if raw.ConnMaxLifetime <= 0 {
		return invalid("database.conn_max_lifetime", "必须大于零")
	}
	return nil
}

func validateTelemetry(raw rawTelemetry) error {
	if raw.OTLPEndpoint == "" {
		return nil
	}
	endpoint, err := url.ParseRequestURI(raw.OTLPEndpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return invalid("telemetry.otlp_endpoint", "必须是绝对 HTTP URL")
	}
	return nil
}

func invalid(field, rule string) error { return &ValidationError{Field: field, Rule: rule} }

func validateArticleImages(raw rawArticleImages, environment Environment) (ArticleImages, error) {
	rawDirectory := strings.TrimSpace(raw.Directory)
	segments := strings.FieldsFunc(rawDirectory, func(character rune) bool { return character == '/' || character == '\\' })
	if slices.Contains(segments, "..") {
		return ArticleImages{}, invalid("article_images.directory", "不得包含上级目录路径")
	}
	directory := filepath.ToSlash(filepath.Clean(rawDirectory))
	if directory == "." || directory == ".." || strings.HasPrefix(directory, "../") {
		return ArticleImages{}, invalid("article_images.directory", "不能为空且不得逃逸工作目录")
	}
	if raw.PendingTTL <= 0 {
		return ArticleImages{}, invalid("article_images.pending_ttl", "必须大于零")
	}
	if raw.OrphanGrace <= 0 {
		return ArticleImages{}, invalid("article_images.orphan_grace", "必须大于零")
	}
	if raw.CleanupInterval <= 0 || raw.CleanupInterval >= raw.PendingTTL || raw.CleanupInterval >= raw.OrphanGrace {
		return ArticleImages{}, invalid("article_images.cleanup_interval", "必须大于零且小于 pending_ttl 和 orphan_grace")
	}
	if raw.MaxFileBytes <= 0 {
		return ArticleImages{}, invalid("article_images.max_file_bytes", "必须大于零")
	}
	if raw.UploadRequestBytes <= raw.MaxFileBytes {
		return ArticleImages{}, invalid("article_images.upload_request_bytes", "必须大于 max_file_bytes")
	}
	if raw.MaxPixels <= 0 {
		return ArticleImages{}, invalid("article_images.max_pixels", "必须大于零")
	}
	if raw.MaxDimension <= 0 {
		return ArticleImages{}, invalid("article_images.max_dimension", "必须大于零")
	}
	if raw.DevelopmentAuthorID != "" {
		if environment != EnvironmentDevelopment {
			return ArticleImages{}, invalid("article_images.development_author_id", "仅 development 环境允许配置固定作者")
		}
		if !validDevelopmentAuthorID(raw.DevelopmentAuthorID) {
			return ArticleImages{}, invalid("article_images.development_author_id", "必须为 32 位小写十六进制")
		}
	}
	return ArticleImages{directory: directory, developmentAuthorID: raw.DevelopmentAuthorID, pendingTTL: raw.PendingTTL, orphanGrace: raw.OrphanGrace, cleanupInterval: raw.CleanupInterval, uploadRequestBytes: raw.UploadRequestBytes, maxFileBytes: raw.MaxFileBytes, maxPixels: raw.MaxPixels, maxDimension: raw.MaxDimension}, nil
}

func validDevelopmentAuthorID(raw string) bool {
	if len(raw) != 32 {
		return false
	}
	for index := range len(raw) {
		character := raw[index]
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
