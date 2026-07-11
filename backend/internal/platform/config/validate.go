package config

import (
	"net"
	"net/url"
	"strings"
	"time"
)

func validate(raw rawConfig) (Config, error) {
	environment := Environment(raw.App.Environment)
	if environment != EnvironmentDevelopment && environment != EnvironmentTest && environment != EnvironmentProduction {
		return Config{}, invalid("app.env", "must be development, test, or production")
	}
	level := LogLevel(raw.App.LogLevel)
	if level != LogLevelDebug && level != LogLevelInfo && level != LogLevelWarn && level != LogLevelError {
		return Config{}, invalid("app.log_level", "must be debug, info, warn, or error")
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
	return Config{
		app:      App{environment: environment, logLevel: level},
		http:     HTTP{host: raw.HTTP.Host, port: raw.HTTP.Port, readHeaderTimeout: raw.HTTP.ReadHeaderTimeout, readTimeout: raw.HTTP.ReadTimeout, writeTimeout: raw.HTTP.WriteTimeout, idleTimeout: raw.HTTP.IdleTimeout, shutdownTimeout: raw.HTTP.ShutdownTimeout, trustedProxies: append([]string(nil), raw.HTTP.TrustedProxies...), corsAllowedOrigins: append([]string(nil), raw.HTTP.CORSAllowedOrigins...)},
		database: Database{dsn: DSN{value: raw.Database.DSN}, maxOpenConns: raw.Database.MaxOpenConns, maxIdleConns: raw.Database.MaxIdleConns, connMaxLifetime: raw.Database.ConnMaxLifetime},
		docs:     Docs{enabled: raw.Docs.Enabled}, telemetry: Telemetry{otlpEndpoint: raw.Telemetry.OTLPEndpoint},
	}, nil
}

func validateHTTP(raw rawHTTP) error {
	if strings.TrimSpace(raw.Host) == "" {
		return invalid("http.host", "must not be empty")
	}
	if raw.Port < 1 || raw.Port > 65535 {
		return invalid("http.port", "must be between 1 and 65535")
	}
	durations := []struct {
		field string
		value time.Duration
	}{{"http.read_header_timeout", raw.ReadHeaderTimeout}, {"http.read_timeout", raw.ReadTimeout}, {"http.write_timeout", raw.WriteTimeout}, {"http.idle_timeout", raw.IdleTimeout}, {"http.shutdown_timeout", raw.ShutdownTimeout}}
	for _, duration := range durations {
		if duration.value <= 0 {
			return invalid(duration.field, "must be positive")
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
				return invalid("http.trusted_proxies", "must contain only IP addresses or CIDRs")
			}
		}
	}
	return nil
}

func validateOrigins(origins []string) error {
	for _, origin := range origins {
		parsed, err := url.ParseRequestURI(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return invalid("http.cors_allowed_origins", "must contain only absolute HTTP origins")
		}
	}
	return nil
}

func validateDatabase(raw rawDatabase) error {
	parsed, err := url.Parse(raw.DSN)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
		return invalid("database.dsn", "must be a PostgreSQL DSN with host and database")
	}
	if raw.MaxOpenConns < 1 {
		return invalid("database.max_open_conns", "must be positive")
	}
	if raw.MaxIdleConns < 0 || raw.MaxIdleConns > raw.MaxOpenConns {
		return invalid("database.max_idle_conns", "must be between zero and max_open_conns")
	}
	if raw.ConnMaxLifetime <= 0 {
		return invalid("database.conn_max_lifetime", "must be positive")
	}
	return nil
}

func validateTelemetry(raw rawTelemetry) error {
	if raw.OTLPEndpoint == "" {
		return nil
	}
	endpoint, err := url.ParseRequestURI(raw.OTLPEndpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return invalid("telemetry.otlp_endpoint", "must be an absolute HTTP URL")
	}
	return nil
}

func invalid(field, rule string) error { return &ValidationError{Field: field, Rule: rule} }
