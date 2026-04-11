package observability

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
)

// ShutdownFunc flushes pending spans and closes the exporter. A no-op
// shutdown is returned when tracing is disabled or the exporter failed to
// construct, so callers can always defer the return value safely.
type ShutdownFunc func(context.Context) error

// InitTracing initializes the OTel TracerProvider from the given Config.
// When cfg.TracingEnabled is false, a no-op provider is used and the returned
// ShutdownFunc is a no-op.
//
// Behavior matches MMA's middleware/tracing.Init byte-for-byte:
//   - Endpoint defaults to "localhost:4317" when empty; http:// / https:// prefix is stripped.
//   - Service name defaults to "dev-atmosoar-api" when empty.
//   - Exporter construction has a 5-second timeout.
//   - On exporter error the library logs a warning and returns a no-op (does not crash).
//   - Sampler is AlwaysSample (collector handles reduction).
//   - W3C traceparent propagation is installed globally.
func InitTracing(cfg Config, logger *zap.SugaredLogger) ShutdownFunc {
	noop := ShutdownFunc(func(context.Context) error { return nil })

	if !cfg.TracingEnabled {
		if logger != nil {
			logger.Infow("OpenTelemetry tracing disabled (TracingEnabled=false)")
		}
		return noop
	}

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = "dev-atmosoar-api"
	}

	endpoint := cfg.OTLPEndpoint
	if endpoint == "" {
		endpoint = "localhost:4317"
	}
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		if logger != nil {
			logger.Warnw("Failed to create OTLP trace exporter, tracing disabled",
				"endpoint", endpoint, "error", err)
		}
		return noop
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	if logger != nil {
		logger.Infow("OpenTelemetry tracing enabled",
			"endpoint", endpoint, "service", serviceName, "sampler", "AlwaysOn")
	}

	return ShutdownFunc(tp.Shutdown)
}
