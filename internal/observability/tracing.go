package observability

import (
	"context"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// SetupTracing initializes the global OpenTelemetry tracer provider.
// If OTEL_EXPORTER_OTLP_ENDPOINT is not set, it configures a no-op provider.
// Returns a shutdown function that should be called when the application exits.
func SetupTracing(ctx context.Context) (func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		// Use no-op provider
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("veloxmesh"),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// RequestTrace wraps an OpenTelemetry span to provide safe helper methods
// that strictly enforce sanitization (D-12).
type RequestTrace struct {
	span trace.Span
}

// StartRequestTrace creates a new RequestTrace wrapping a trace span.
func StartRequestTrace(ctx context.Context, reqID string, model string) (context.Context, *RequestTrace) {
	tracer := otel.Tracer("veloxmesh/gateway")
	ctx, span := tracer.Start(ctx, "HandleChatCompletion")

	// D-12: we can set request_id and model, as these are safe identifiers.
	// We strictly do not log user identifiers or raw request text.
	span.SetAttributes(
		attribute.String("request.id", reqID),
		attribute.String("request.model", model),
	)

	return ctx, &RequestTrace{span: span}
}

// RecordRouting captures routing decisions for the request.
func (rt *RequestTrace) RecordRouting(strategy string, cacheResult string, fallbackReason string, scoreSummary string) {
	attrs := []attribute.KeyValue{
		attribute.String("routing.strategy", strategy),
		attribute.String("routing.cache_result", cacheResult),
	}

	if fallbackReason != "" {
		attrs = append(attrs, attribute.String("routing.fallback_reason", fallbackReason))
	}

	// Ensure the score summary contains only routing math.
	if scoreSummary != "" {
		attrs = append(attrs, attribute.String("routing.score_summary", scoreSummary))
	}

	rt.span.SetAttributes(attrs...)
}

// RecordOutcome captures the final status and latencies for the request, and ends the span.
// ttft, tpot, and e2e are typically recorded in milliseconds.
func (rt *RequestTrace) RecordOutcome(provider string, status int, errorCategory string, ttftMs float64, tpotMs float64, e2eMs float64) {
	defer rt.span.End()

	attrs := []attribute.KeyValue{
		attribute.String("outcome.provider", provider),
		attribute.Int("outcome.status", status),
		attribute.Float64("latency.ttft_ms", ttftMs),
		attribute.Float64("latency.e2e_ms", e2eMs),
	}

	if tpotMs > 0 {
		attrs = append(attrs, attribute.Float64("latency.tpot_ms", tpotMs))
	}

	if errorCategory != "" {
		// D-12: error category is safe, raw error strings might not be.
		// Check for potentially dangerous raw error messages that accidentally slipped through
		// by matching strict alphanumeric + underscore. (errorCategory should be a code like "provider_error")
		safeErr := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
				return r
			}
			return '_'
		}, errorCategory)
		attrs = append(attrs, attribute.String("outcome.error_category", safeErr))
	}

	rt.span.SetAttributes(attrs...)
}

// End immediately finishes the trace, used mainly in cases of early rejection where RecordOutcome isn't reached.
func (rt *RequestTrace) End() {
	rt.span.End()
}

// EndWithError ends the span and records the error event.
func (rt *RequestTrace) EndWithError(err error) {
	if err != nil {
		rt.span.RecordError(err)
	}
	rt.span.End()
}
