// Package config loads and validates immutable startup configuration.
package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// Environment identifies a supported runtime environment.
type Environment string

// Supported runtime environments.
const (
	EnvironmentDevelopment Environment = "development"
	EnvironmentTest        Environment = "test"
	EnvironmentProduction  Environment = "production"
)

// LogLevel identifies a supported slog threshold.
type LogLevel string

// Supported log levels.
const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// DSN stores a validated database connection string without exposing it through formatting.
type DSN struct{ value string }

// String returns a safe representation of the database DSN.
func (DSN) String() string { return "[REDACTED]" }

// Value returns the validated DSN for the database adapter only.
func (dsn DSN) Value() string { return dsn.value }

// MarshalJSON prevents accidental credential serialization.
func (DSN) MarshalJSON() ([]byte, error) { return json.Marshal("[REDACTED]") }

// App is an immutable application configuration value.
type App struct {
	environment Environment
	logLevel    LogLevel
}

// Environment returns the validated runtime environment.
func (app App) Environment() Environment { return app.environment }

// LogLevel returns the validated structured logging threshold.
func (app App) LogLevel() LogLevel { return app.logLevel }

// HTTP is an immutable HTTP server configuration value.
type HTTP struct {
	host               string
	trustedProxies     []string
	corsAllowedOrigins []string
	port               int
	readHeaderTimeout  time.Duration
	readTimeout        time.Duration
	writeTimeout       time.Duration
	idleTimeout        time.Duration
	shutdownTimeout    time.Duration
}

// Host returns the configured bind host.
func (http HTTP) Host() string { return http.host }

// Port returns the validated bind port.
func (http HTTP) Port() int { return http.port }

// ReadHeaderTimeout returns the header read deadline.
func (http HTTP) ReadHeaderTimeout() time.Duration { return http.readHeaderTimeout }

// ReadTimeout returns the request read deadline.
func (http HTTP) ReadTimeout() time.Duration { return http.readTimeout }

// WriteTimeout returns the response write deadline.
func (http HTTP) WriteTimeout() time.Duration { return http.writeTimeout }

// IdleTimeout returns the keep-alive idle deadline.
func (http HTTP) IdleTimeout() time.Duration { return http.idleTimeout }

// ShutdownTimeout returns the graceful shutdown deadline.
func (http HTTP) ShutdownTimeout() time.Duration { return http.shutdownTimeout }

// TrustedProxies returns an independent copy of validated proxies.
func (http HTTP) TrustedProxies() []string { return append([]string(nil), http.trustedProxies...) }

// CORSAllowedOrigins returns an independent copy of strict origins.
func (http HTTP) CORSAllowedOrigins() []string {
	return append([]string(nil), http.corsAllowedOrigins...)
}

// Database is an immutable database pool configuration value.
type Database struct {
	dsn             DSN
	connMaxLifetime time.Duration
	maxOpenConns    int
	maxIdleConns    int
}

// DSN returns the validated secret-bearing connection value.
func (database Database) DSN() DSN { return database.dsn }

// MaxOpenConns returns the maximum open pool size.
func (database Database) MaxOpenConns() int { return database.maxOpenConns }

// MaxIdleConns returns the maximum idle pool size.
func (database Database) MaxIdleConns() int { return database.maxIdleConns }

// ConnMaxLifetime returns the connection lifetime limit.
func (database Database) ConnMaxLifetime() time.Duration { return database.connMaxLifetime }

// Docs is an immutable API documentation configuration value.
type Docs struct{ enabled bool }

// Enabled reports whether API documentation is enabled.
func (docs Docs) Enabled() bool { return docs.enabled }

// Telemetry is an immutable telemetry configuration value.
type Telemetry struct{ otlpEndpoint string }

// OTLPEndpoint returns the validated optional collector URL.
func (telemetry Telemetry) OTLPEndpoint() string { return telemetry.otlpEndpoint }

// Config is an immutable, validated startup configuration value.
type Config struct {
	app       App
	telemetry Telemetry
	database  Database
	http      HTTP
	docs      Docs
}

// App returns immutable application settings.
func (config Config) App() App { return config.app }

// HTTP returns immutable HTTP settings.
func (config Config) HTTP() HTTP { return config.http }

// Database returns immutable database settings.
func (config Config) Database() Database { return config.database }

// Docs returns immutable documentation settings.
func (config Config) Docs() Docs { return config.docs }

// Telemetry returns immutable telemetry settings.
func (config Config) Telemetry() Telemetry { return config.telemetry }

// String returns a deterministic secret-free summary.
func (config Config) String() string {
	return fmt.Sprintf("env=%s host=%s port=%d database=[REDACTED]", config.app.environment, config.http.host, config.http.port)
}
