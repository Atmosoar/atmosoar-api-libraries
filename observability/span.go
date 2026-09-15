package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// tracerPrefix namespaces the tracers this package hands out.
const tracerPrefix = "atmosoar-api-libraries/observability/"

// StartSpan starts a span for work that does not arrive over HTTP — an MQTT
// publish, a TCP frame, a UDP datagram — so it lands in the same trace backend
// as HTTP requests. component names the tracer
// ("atmosoar-api-libraries/observability/<component>") and name the span.
// The caller must End the returned span.
func StartSpan(
	ctx context.Context,
	component, name string,
	kind trace.SpanKind,
	attrs ...attribute.KeyValue,
) (context.Context, trace.Span) {
	return otel.Tracer(tracerPrefix+component).Start(ctx, name,
		trace.WithSpanKind(kind),
		trace.WithAttributes(attrs...),
	)
}
