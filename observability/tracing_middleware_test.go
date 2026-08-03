package observability

import (
	"net/http"
	"net/url"
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

func TestRedactedURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"no query untouched", "/impact/v1/stream", "/impact/v1/stream"},
		{"access_token redacted", "/impact/v1/stream?access_token=eyJhbGciOi.secret.sig&days=3",
			"/impact/v1/stream?access_token=REDACTED&days=3"},
		{"case-insensitive param name", "/x?Access_Token=secret", "/x?Access_Token=REDACTED"},
		{"other params preserved", "/x?location=1,2&token=abc", "/x?location=1%2C2&token=REDACTED"},
		{"benign query untouched verbatim", "/x?location=1,2", "/x?location=1,2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := redactedURL(u); got != tc.want {
				t.Errorf("redactedURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
	if got := redactedURL(nil); got != "" {
		t.Errorf("redactedURL(nil) = %q, want empty", got)
	}
}
