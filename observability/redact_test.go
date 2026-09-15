package observability

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func maskUploadToken(p string) string {
	const prefix = "/stations/ingest/ecowitt/"
	if strings.HasPrefix(p, prefix) {
		return prefix + "[REDACTED]"
	}
	return p
}

func TestRedactURL_StationCredentials(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "wunderground password",
			raw:  "http://h/weatherstation/updateweatherstation.php?ID=st1&PASSWORD=hunter2&tempf=50",
			want: "http://h/weatherstation/updateweatherstation.php?ID=st1&PASSWORD=REDACTED&tempf=50",
		},
		{
			name: "ecowitt passkey",
			raw:  "http://h/data?PASSKEY=abc&humidity=40",
			want: "http://h/data?PASSKEY=REDACTED&humidity=40",
		},
		{
			name: "userinfo password",
			raw:  "mqtt://station:s3cret@broker:1883/topic",
			want: "mqtt://station:REDACTED@broker:1883/topic",
		},
		{
			name: "nothing sensitive stays byte-identical",
			raw:  "http://h/path?b=2&a=1",
			want: "http://h/path?b=2&a=1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.raw)
			require.NoError(t, err)
			assert.Equal(t, tc.want, RedactURL(u))
		})
	}
}

func TestRedactURLWithPath(t *testing.T) {
	u, err := url.Parse("http://h/stations/ingest/ecowitt/ABCDEFGHIJKLMNOP?PASSKEY=x")
	require.NoError(t, err)
	assert.Equal(t, "http://h/stations/ingest/ecowitt/%5BREDACTED%5D?PASSKEY=REDACTED",
		RedactURLWithPath(u, maskUploadToken))
	assert.Equal(t, "http://h/stations/ingest/ecowitt/ABCDEFGHIJKLMNOP", RedactURLWithPath(
		&url.URL{Scheme: "http", Host: "h", Path: "/stations/ingest/ecowitt/ABCDEFGHIJKLMNOP"}, nil),
		"a nil redactor leaves the path alone")
	assert.Empty(t, RedactURLWithPath(nil, maskUploadToken))
}

func TestTracingMiddleware_WithPathRedactor(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TracingMiddleware(nil, WithPathRedactor(maskUploadToken)))
	router.POST("/stations/ingest/ecowitt/:token", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/stations/ingest/ecowitt/ABCDEFGHIJKLMNOP", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	for _, kv := range spans[0].Attributes() {
		if kv.Key == "http.url" {
			assert.NotContains(t, kv.Value.AsString(), "ABCDEFGHIJKLMNOP")
			return
		}
	}
	t.Fatal("span has no http.url attribute")
}

func TestStartSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	ctx, span := StartSpan(t.Context(), "mqtt", "mqtt publish", trace.SpanKindServer,
		attribute.String("messaging.system", "mqtt"))
	assert.True(t, trace.SpanFromContext(ctx).SpanContext().IsValid())
	span.End()

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "mqtt publish", spans[0].Name())
	assert.Equal(t, trace.SpanKindServer, spans[0].SpanKind())
	assert.Equal(t, "atmosoar-api-libraries/observability/mqtt", spans[0].InstrumentationScope().Name)
}
