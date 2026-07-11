package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
)

var secretSentinel = "task3-" + "secret-sentinel"

func Test_Logger_production_serializes_error_and_correlation_fields_without_secret(t *testing.T) {
	// Given
	var output bytes.Buffer
	logger, err := observability.NewLogger(observability.LoggerOptions{Environment: "production", Level: "error", Writer: &output})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	ctx := observability.WithRequestFields(context.Background(), observability.RequestFields{
		RequestID: "req-1", CorrelationID: "corr-1", CausationID: "cause-1",
	})

	// When
	logger.LogAttrs(ctx, slog.LevelError, "operation.failed", observability.ContextAttrs(ctx)...)
	logger.LogAttrs(ctx, slog.LevelError, "database.failed", observability.Error(errors.New("failed "+secretSentinel)))

	// Then
	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("lines=%d output=%s", len(lines), output.String())
	}
	var record map[string]json.RawMessage
	if decodeErr := json.Unmarshal(lines[0], &record); decodeErr != nil {
		t.Fatalf("json log: %v", decodeErr)
	}
	for _, key := range []string{"request_id", "correlation_id", "causation_id", "level", "msg"} {
		if _, ok := record[key]; !ok {
			t.Fatalf("missing key %q in %s", key, lines[0])
		}
	}
	if !bytes.Contains(lines[1], []byte(`"error"`)) {
		t.Fatalf("error not serialized: %s", lines[1])
	}
	if strings.Contains(output.String(), secretSentinel) {
		t.Fatal("logger leaked sentinel")
	}
}

func Test_Logger_context_attrs_include_trace_and_span(t *testing.T) {
	// Given
	ctx := observability.WithTraceFields(context.Background(), observability.TraceFields{TraceID: "trace-1", SpanID: "span-1"})

	// When
	attrs := observability.ContextAttrs(ctx)

	// Then
	text := attrsText(attrs)
	if !strings.Contains(text, "trace_id=trace-1") || !strings.Contains(text, "span_id=span-1") {
		t.Fatalf("attrs=%s", text)
	}
}

func Test_Logger_private_registry_does_not_use_global_default(t *testing.T) {
	// Given
	before, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather default: %v", err)
	}

	// When
	metrics := observability.NewMetrics()
	metrics.ObserveRequest("GET", "articles", "2xx")
	gathered, err := metrics.Registry().Gather()
	// Then
	if err != nil {
		t.Fatalf("gather private: %v", err)
	}
	after, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather default after: %v", err)
	}
	if len(gathered) == 0 {
		t.Fatal("private registry is empty")
	}
	if len(after) != len(before) {
		t.Fatal("default registry was mutated")
	}
}

func Test_Logger_telemetry_shutdown_is_idempotent(t *testing.T) {
	// Given
	lifecycle := observability.NewTelemetry()

	// When / Then
	if err := lifecycle.Shutdown(context.Background()); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	if err := lifecycle.Shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
}

func Test_Health_live_only_reflects_process_and_ready_is_revocable(t *testing.T) {
	// Given
	dbOK, migrationOK := true, true
	health, constructorErr := observability.NewHealth(
		func(context.Context) error {
			if !dbOK {
				return errors.New("db unavailable")
			}
			return nil
		},
		func(context.Context) error {
			if !migrationOK {
				return errors.New("migration pending")
			}
			return nil
		},
	)
	if constructorErr != nil {
		t.Fatalf("new health: %v", constructorErr)
	}

	// When / Then
	if !health.Live() {
		t.Fatal("process should be live")
	}
	if ready, err := health.Ready(context.Background()); !ready || err != nil {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	dbOK = false
	if ready, err := health.Ready(context.Background()); ready || err == nil {
		t.Fatalf("db failure ready=%v err=%v", ready, err)
	}
	dbOK, migrationOK = true, false
	if ready, err := health.Ready(context.Background()); ready || err == nil {
		t.Fatalf("migration failure ready=%v err=%v", ready, err)
	}
	health.Withdraw()
	migrationOK = true
	if ready, err := health.Ready(context.Background()); ready || !errors.Is(err, observability.ErrShuttingDown) {
		t.Fatalf("shutdown ready=%v err=%v", ready, err)
	}
	if !health.Live() {
		t.Fatal("liveness must remain process-only")
	}
}

func attrsText(attrs []slog.Attr) string {
	var parts []string
	for _, attr := range attrs {
		parts = append(parts, attr.String())
	}
	return strings.Join(parts, " ")
}

func Test_Logger_rejects_nil_writer_with_typed_error(t *testing.T) {
	// Given / When
	_, err := observability.NewLogger(observability.LoggerOptions{Environment: "production", Level: "info"})

	// Then
	if err == nil {
		t.Fatal("expected writer validation error")
	}
}

func Test_Logger_rejects_unknown_environment_with_typed_error(t *testing.T) {
	// Given / When
	_, err := observability.NewLogger(observability.LoggerOptions{Environment: "staging", Level: "info", Writer: &bytes.Buffer{}})

	// Then
	if err == nil {
		t.Fatal("expected environment validation error")
	}
}

func Test_Logger_Error_nil_is_safe_and_serializable(t *testing.T) {
	// Given
	var output bytes.Buffer
	logger, err := observability.NewLogger(observability.LoggerOptions{Environment: "production", Level: "error", Writer: &output})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	// When
	logger.LogAttrs(context.Background(), slog.LevelError, "operation.failed", observability.Error(nil))

	// Then
	var record map[string]json.RawMessage
	if decodeErr := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); decodeErr != nil {
		t.Fatalf("decode JSON error log: %v", decodeErr)
	}
	if _, exists := record["error"]; !exists {
		t.Fatal("nil error attr was not serialized")
	}
}

func Test_Health_nil_probe_returns_error_instead_of_panicking(t *testing.T) {
	// Given / When
	health, err := observability.NewHealth(nil, func(context.Context) error { return nil })

	// Then
	var optionsErr *observability.OptionsError
	if health != nil || !errors.As(err, &optionsErr) || optionsErr.Field() != "database_probe" {
		t.Fatalf("health=%v error=%T: %v", health, err, err)
	}
}

func Test_Telemetry_canceled_shutdown_does_not_consume_lifecycle(t *testing.T) {
	// Given
	lifecycle := observability.NewTelemetry()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	// When
	canceledErr := lifecycle.Shutdown(canceled)
	validErr := lifecycle.Shutdown(context.Background())

	// Then
	if !errors.Is(canceledErr, context.Canceled) {
		t.Fatalf("canceled shutdown error=%v", canceledErr)
	}
	if validErr != nil {
		t.Fatalf("valid shutdown after cancellation: %v", validErr)
	}
}

func Test_Health_rejects_nil_migration_probe(t *testing.T) {
	// Given / When
	health, err := observability.NewHealth(func(context.Context) error { return nil }, nil)

	// Then
	var optionsErr *observability.OptionsError
	if health != nil || !errors.As(err, &optionsErr) || optionsErr.Field() != "migration_probe" {
		t.Fatalf("health=%v error=%T: %v", health, err, err)
	}
}

func Test_Health_cancellation_stops_before_database_probe(t *testing.T) {
	// Given
	calls := 0
	health, err := observability.NewHealth(
		func(context.Context) error { calls++; return nil },
		func(context.Context) error { calls++; return nil },
	)
	if err != nil {
		t.Fatalf("new health: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When
	ready, readyErr := health.Ready(ctx)

	// Then
	if ready || !errors.Is(readyErr, context.Canceled) || calls != 0 {
		t.Fatalf("ready=%v err=%v calls=%d", ready, readyErr, calls)
	}
}

func Test_Health_database_failure_prevents_migration_probe(t *testing.T) {
	// Given
	migrationCalls := 0
	databaseErr := errors.New("database unavailable")
	health, err := observability.NewHealth(
		func(context.Context) error { return databaseErr },
		func(context.Context) error { migrationCalls++; return nil },
	)
	if err != nil {
		t.Fatalf("new health: %v", err)
	}

	// When
	ready, readyErr := health.Ready(context.Background())

	// Then
	if ready || !errors.Is(readyErr, databaseErr) || migrationCalls != 0 {
		t.Fatalf("ready=%v err=%v migration_calls=%d", ready, readyErr, migrationCalls)
	}
}

func Test_Telemetry_successful_shutdown_is_idempotent_after_cancellation_test(t *testing.T) {
	// Given
	lifecycle := observability.NewTelemetry()

	// When
	firstErr := lifecycle.Shutdown(context.Background())
	secondErr := lifecycle.Shutdown(context.Background())

	// Then
	if firstErr != nil || secondErr != nil {
		t.Fatalf("first=%v second=%v", firstErr, secondErr)
	}
}

func Test_Logger_rejects_typed_nil_writer_with_typed_error(t *testing.T) {
	// Given
	var writer *bytes.Buffer

	// When
	logger, err := observability.NewLogger(observability.LoggerOptions{Writer: writer, Environment: "production", Level: "info"})

	// Then
	var optionsErr *observability.OptionsError
	if logger != nil || !errors.As(err, &optionsErr) || optionsErr.Field() != "writer" {
		t.Fatalf("logger=%v error=%T: %v", logger, err, err)
	}
}
