package observability

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TracingMiddleware returns a Gin middleware that creates a root span for
// each request. Spans carry http.method, http.route, http.url, and
// http.status_code attributes, and are marked Error for 5xx responses.
//
// skipPaths are request paths excluded from span creation (to avoid noise
// from infrastructure probes). Pass nil to trace all paths.
func TracingMiddleware(skipPaths []string) gin.HandlerFunc {
	tracer := otel.Tracer("atmosoar-api-libraries/observability/http")

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

		ctx, span := tracer.Start(c.Request.Context(), spanName,
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
