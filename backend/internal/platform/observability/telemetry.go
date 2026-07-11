package observability

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Telemetry owns a non-global OpenTelemetry API provider and lifecycle state.
type Telemetry struct {
	provider trace.TracerProvider
	mutex    sync.Mutex
	stopped  bool
}

// NewTelemetry constructs an explicit non-global no-op provider.
func NewTelemetry() *Telemetry { return &Telemetry{provider: noop.NewTracerProvider()} }

// Tracer returns a tracer owned by this provider.
func (telemetry *Telemetry) Tracer(name string) trace.Tracer { return telemetry.provider.Tracer(name) }

// Shutdown honors cancellation without consuming a later valid shutdown.
func (telemetry *Telemetry) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	telemetry.mutex.Lock()
	defer telemetry.mutex.Unlock()
	if telemetry.stopped {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	telemetry.stopped = true
	return nil
}
