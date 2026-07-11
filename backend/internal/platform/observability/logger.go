// Package observability provides structured logging, metrics, tracing lifecycle, and health contracts.
package observability

import (
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
)

const errorKey = "error"

// OptionsError reports an invalid observability constructor option.
type OptionsError struct {
	field string
	rule  string
}

// Error returns a value-free option validation description.
func (err *OptionsError) Error() string {
	return fmt.Sprintf("observability option %s: %s", err.field, err.rule)
}

// Field returns the invalid option name.
func (err *OptionsError) Field() string { return err.field }

// LoggerOptions selects a handler without mutating slog's global logger.
type LoggerOptions struct {
	// Writer receives complete structured records.
	Writer io.Writer
	// Environment selects development, test, or production formatting.
	Environment string
	// Level selects debug, info, warn, or error threshold.
	Level string
}

// NewLogger constructs the service's only logger after validating every option.
func NewLogger(options LoggerOptions) (*slog.Logger, error) {
	if writerIsNil(options.Writer) {
		return nil, &OptionsError{field: "writer", rule: "must not be nil"}
	}
	if options.Environment != "development" && options.Environment != "test" && options.Environment != "production" {
		return nil, &OptionsError{field: "environment", rule: "must be development, test, or production"}
	}
	level, err := parseLevel(options.Level)
	if err != nil {
		return nil, err
	}
	handlerOptions := &slog.HandlerOptions{Level: level, ReplaceAttr: redactAttr}
	if options.Environment == "production" {
		return slog.New(slog.NewJSONHandler(options.Writer, handlerOptions)), nil
	}
	return slog.New(slog.NewTextHandler(options.Writer, handlerOptions)), nil
}

func writerIsNil(writer io.Writer) bool {
	if writer == nil {
		return true
	}
	// An interface can be non-nil while holding a nil pointer-like dynamic value.
	value := reflect.ValueOf(writer)
	kind := value.Kind()
	nilable := kind == reflect.Chan || kind == reflect.Func || kind == reflect.Interface || kind == reflect.Map || kind == reflect.Pointer || kind == reflect.Slice
	return nilable && value.IsNil()
}

func parseLevel(value string) (slog.Level, error) {
	switch value {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, &OptionsError{field: "level", rule: "must be debug, info, warn, or error"}
	}
}

func redactAttr(_ []string, attr slog.Attr) slog.Attr {
	lowered := strings.ToLower(attr.Key)
	if strings.Contains(lowered, "password") || strings.Contains(lowered, "secret") || strings.Contains(lowered, "authorization") || strings.Contains(lowered, "dsn") {
		attr.Value = slog.StringValue("[REDACTED]")
		return attr
	}
	if attr.Value.Kind() == slog.KindString {
		attr.Value = slog.StringValue(redactText(attr.Value.String()))
	}
	if attr.Value.Kind() == slog.KindAny {
		if err, ok := attr.Value.Any().(error); ok {
			attr.Value = slog.StringValue(redactText(err.Error()))
		}
	}
	return attr
}

func redactText(value string) string {
	lowered := strings.ToLower(value)
	if strings.Contains(lowered, "secret") || strings.Contains(lowered, "password") || strings.Contains(lowered, "postgres://") || strings.Contains(lowered, "postgresql://") {
		return "[REDACTED]"
	}
	return value
}

// Error returns the standard error attribute; nil is serialized safely.
func Error(err error) slog.Attr {
	if err == nil {
		return slog.String(errorKey, "<nil>")
	}
	return slog.String(errorKey, redactText(err.Error()))
}
