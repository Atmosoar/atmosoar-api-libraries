package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// A fixed, valid W3C traceparent — trace-id 11111111111111111111111111111111.
const remoteTraceparent = "00-11111111111111111111111111111111-2222222222222222-01"
const remoteTraceID = "11111111111111111111111111111111"

func setupTestTracing(t *testing.T) {
	t.Helper()
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})
}

func captureTraceID(mw gin.HandlerFunc) string {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mw)
	var got string
	r.GET("/x", func(c *gin.Context) {
		got = trace.SpanFromContext(c.Request.Context()).SpanContext().TraceID().String()
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("traceparent", remoteTraceparent)
	r.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

// Default middleware joins the caller's trace (continues remoteTraceID).
func TestTracingMiddleware_ExtractsRemoteContextByDefault(t *testing.T) {
	setupTestTracing(t)
	if got := captureTraceID(TracingMiddleware(nil)); got != remoteTraceID {
		t.Fatalf("expected span to continue remote trace %s, got %s", remoteTraceID, got)
	}
}

// WithoutRemoteExtraction starts a fresh root, ignoring the incoming header.
func TestTracingMiddleware_WithoutRemoteExtraction(t *testing.T) {
	setupTestTracing(t)
	got := captureTraceID(TracingMiddleware(nil, WithoutRemoteExtraction()))
	if got == remoteTraceID {
		t.Fatalf("expected a fresh root trace, but span continued the remote trace %s", got)
	}
	if got == "00000000000000000000000000000000" || got == "" {
		t.Fatalf("expected a valid new trace id, got %q", got)
	}
}
