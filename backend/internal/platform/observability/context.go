package observability

import (
	"context"
	"log/slog"
)

type contextKey uint8

const (
	fieldsKey        contextKey = iota
	requestIDKey                = "request_id"
	correlationIDKey            = "correlation_id"
	causationIDKey              = "causation_id"
	traceIDKey                  = "trace_id"
	spanIDKey                   = "span_id"
)

// RequestFields are stable identifiers propagated across request logs.
type RequestFields struct {
	// RequestID uniquely identifies one inbound request within the service.
	RequestID string
	// CorrelationID groups operations that belong to one distributed business interaction.
	CorrelationID string
	// CausationID identifies the command or event that directly caused this request.
	CausationID string
}

// TraceFields are W3C trace identifiers propagated into structured logs.
type TraceFields struct {
	// TraceID is the W3C trace identifier shared across the distributed trace.
	TraceID string
	// SpanID identifies the current span within the W3C trace.
	SpanID string
}

type fields struct {
	request RequestFields
	trace   TraceFields
}

// WithRequestFields returns a context containing immutable request identifiers.
func WithRequestFields(ctx context.Context, request RequestFields) context.Context {
	current := fieldsFromContext(ctx)
	current.request = request
	return context.WithValue(ctx, fieldsKey, current)
}

// WithTraceFields returns a context containing immutable trace identifiers.
func WithTraceFields(ctx context.Context, trace TraceFields) context.Context {
	current := fieldsFromContext(ctx)
	current.trace = trace
	return context.WithValue(ctx, fieldsKey, current)
}

// ContextAttrs returns canonical slog attributes without empty values.
func ContextAttrs(ctx context.Context) []slog.Attr {
	current := fieldsFromContext(ctx)
	candidates := []slog.Attr{
		slog.String(requestIDKey, current.request.RequestID), slog.String(correlationIDKey, current.request.CorrelationID),
		slog.String(causationIDKey, current.request.CausationID), slog.String(traceIDKey, current.trace.TraceID), slog.String(spanIDKey, current.trace.SpanID),
	}
	attrs := make([]slog.Attr, 0, len(candidates))
	for _, attr := range candidates {
		if attr.Value.String() != "" {
			attrs = append(attrs, attr)
		}
	}
	return attrs
}

// RequestIDFromContext returns the trusted inbound request identifier when present.
func RequestIDFromContext(ctx context.Context) string {
	return fieldsFromContext(ctx).request.RequestID
}

func fieldsFromContext(ctx context.Context) fields {
	value, ok := ctx.Value(fieldsKey).(fields)
	if !ok {
		return fields{}
	}
	return value
}
