package observability

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// tracingConfig holds TracingMiddleware options.
type tracingConfig struct {
	extractRemote bool
}

// TracingOption configures TracingMiddleware.
type TracingOption func(*tracingConfig)

// WithoutRemoteExtraction disables joining an incoming (remote) trace context
// from request headers. Use it at a trust boundary such as the public gateway,
// where a client-supplied traceparent must not be honored. Internal services
// should leave extraction on (the default) so they continue the caller's trace
// instead of starting a disconnected root span.
func WithoutRemoteExtraction() TracingOption {
	return func(c *tracingConfig) { c.extractRemote = false }
}

// TracingMiddleware returns a Gin middleware that creates a span for each
// request. By default it extracts the incoming W3C trace context from the
// request headers so the span continues the caller's trace; pass
// WithoutRemoteExtraction to start a fresh root instead. Spans carry
// http.method, http.route, http.url, and http.status_code attributes, and are
// marked Error for 5xx responses.
//
// skipPaths are request paths excluded from span creation (to avoid noise
// from infrastructure probes). Pass nil to trace all paths.
func TracingMiddleware(skipPaths []string, opts ...TracingOption) gin.HandlerFunc {
	tracer := otel.Tracer("atmosoar-api-libraries/observability/http")

	cfg := tracingConfig{extractRemote: true}
	for _, o := range opts {
		o(&cfg)
	}

	skip := make(map[string]bool, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = true
	}

	return func(c *gin.Context) {
		if skip[c.Request.URL.Path] {
			c.Next()
			return
		}

		// Use route template for span name; fall back to method + path.
		route := c.FullPath()
		if route == "" {
			route = "/unknown"
		}
		spanName := fmt.Sprintf("%s %s", c.Request.Method, route)

		// Join the caller's trace when a remote parent is present. Extract is a
		// no-op (returns the same context) when there is no incoming
		// traceparent or the global propagator is the no-op one, so a directly
		// called or untraced service still gets a clean root span.
		startCtx := c.Request.Context()
		if cfg.extractRemote {
			startCtx = otel.GetTextMapPropagator().Extract(
				startCtx, propagation.HeaderCarrier(c.Request.Header),
			)
		}

		ctx, span := tracer.Start(startCtx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
		)

		// Inject trace context into the request so downstream code (adapters)
		// can access it.
		c.Request = c.Request.WithContext(ctx)

		span.SetAttributes(
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.route", route),
			attribute.String("http.url", c.Request.URL.String()),
		)

		c.Next()

		status := c.Writer.Status()
		span.SetAttributes(attribute.Int("http.status_code", status))

		if status >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
		}

		span.End()
	}
}
