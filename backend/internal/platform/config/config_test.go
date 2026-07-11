package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
)

var secretSentinel = "task3-" + "secret-sentinel"

func Test_Config_defaults_file_environment_precedence(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "app:\n  env: production\n  log_level: warn\nhttp:\n  host: file-host\n  port: 9090\ndatabase:\n  dsn: postgres://file:" + "pass" + "word@localhost:5432/blog?sslmode=disable\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCYG_HTTP_HOST", "env-host")
	t.Setenv("SCYG_DATABASE_DSN", "postgres://env:"+secretSentinel+"@localhost:5432/blog?sslmode=disable")

	// When
	cfg, err := config.Load(config.Options{File: path})
	// Then
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.HTTP().Host() != "env-host" || cfg.HTTP().Port() != 9090 {
		t.Fatalf("unexpected http config: %+v", cfg.HTTP())
	}
	if cfg.App().Environment() != config.EnvironmentProduction || cfg.App().LogLevel() != config.LogLevelWarn {
		t.Fatalf("unexpected app config: %+v", cfg.App())
	}
	if cfg.Database().DSN().String() != "[REDACTED]" {
		t.Fatalf("dsn String leaked: %s", cfg.Database().DSN().String())
	}
	if strings.Contains(cfg.String(), secretSentinel) {
		t.Fatal("config String leaked sentinel")
	}
}

func Test_Config_defaults_apply_without_optional_file(t *testing.T) {
	// Given / When
	cfg, err := config.Load(config.Options{})
	// Then
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	if cfg.HTTP().Port() != 8080 || cfg.HTTP().ReadTimeout() != 15*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg.HTTP())
	}
	if cfg.Docs().Enabled() != true {
		t.Fatal("docs should default enabled")
	}
}

func Test_Config_invalid_values_fail_in_stable_order_without_secrets(t *testing.T) {
	tests := []struct{ name, key, value, field string }{
		{"dsn", "SCYG_DATABASE_DSN", "postgres://user:" + secretSentinel + "@", "database.dsn"},
		{"proxy", "SCYG_HTTP_TRUSTED_PROXIES", "not-a-proxy", "http.trusted_proxies"},
		{"log level", "SCYG_APP_LOG_LEVEL", "verbose", "app.log_level"},
		{"duration", "SCYG_HTTP_READ_TIMEOUT", "0s", "http.read_timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			t.Setenv(tt.key, tt.value)

			// When
			_, err := config.Load(config.Options{})

			// Then
			if err == nil {
				t.Fatal("expected validation error")
			}
			var validationErr *config.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError, got %T: %v", err, err)
			}
			if validationErr.Field != tt.field {
				t.Fatalf("field=%q want %q", validationErr.Field, tt.field)
			}
			if strings.Contains(err.Error(), secretSentinel) {
				t.Fatal("error leaked sentinel")
			}
		})
	}
}

func Test_Config_explicit_broken_file_returns_contextual_typed_error(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "broken.yaml")
	if err := os.WriteFile(path, []byte("http: ["), 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	_, err := config.Load(config.Options{File: path})

	// Then
	var fileErr *config.FileError
	if !errors.As(err, &fileErr) {
		t.Fatalf("expected FileError, got %T: %v", err, err)
	}
	if fileErr.Path != path {
		t.Fatalf("path=%q want %q", fileErr.Path, path)
	}
}

func Test_Config_rejects_non_origin_CORS_values(t *testing.T) {
	invalidOrigins := []string{
		"https://example.com/path",
		"https://example.com?query=value",
		"https://user@example.com",
		"https://example.com#fragment",
	}
	for _, origin := range invalidOrigins {
		t.Run(origin, func(t *testing.T) {
			// Given
			t.Setenv("SCYG_HTTP_CORS_ALLOWED_ORIGINS", origin)

			// When
			_, err := config.Load(config.Options{})

			// Then
			var validationErr *config.ValidationError
			if !errors.As(err, &validationErr) || validationErr.Field != "http.cors_allowed_origins" {
				t.Fatalf("origin %q accepted or wrong error: %v", origin, err)
			}
		})
	}
}

func Test_Config_public_values_expose_no_mutable_fields(t *testing.T) {
	// Given
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}

	// When / Then
	for _, value := range []any{cfg.App(), cfg.HTTP(), cfg.Database(), cfg.Docs(), cfg.Telemetry()} {
		typeOfValue := reflect.TypeOf(value)
		for index := range typeOfValue.NumField() {
			if typeOfValue.Field(index).IsExported() {
				t.Fatalf("%s exposes mutable field %s", typeOfValue, typeOfValue.Field(index).Name)
			}
		}
	}
}
