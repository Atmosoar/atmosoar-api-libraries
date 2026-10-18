package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// panicRouter mounts mw behind gin.Recovery (outermost, as radar-airspace-api,
// observations-api and elevation-api register it) with a handler that panics.
func panicRouter(mw gin.HandlerFunc, path string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery(), mw)
	r.GET(path, func(*gin.Context) { panic("nil upstream data") })
	return r
}

// metricValue gathers the library collectors through a private registry and
// returns the value of the named series whose labels contain want (gauge or
// counter), or 0 when the series does not exist yet.
func metricValue(t *testing.T, name string, want map[string]string) float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(HTTPRequestsInFlight, HTTPRequestsTotal)
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
	metrics:
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			for k, v := range want {
				if labels[k] != v {
					continue metrics
				}
			}
			if g := m.GetGauge(); g != nil {
				return g.GetValue()
			}
			return m.GetCounter().GetValue()
		}
	}
	return 0
}

func TestHTTPMetricsMiddleware_PanicReleasesInFlightAndCounts500(t *testing.T) {
	const path = "/panic-metrics"
	r := panicRouter(HTTPMetricsMiddleware(""), path)
	series := map[string]string{"method": http.MethodGet, "path": path, "status": "500"}
	before := metricValue(t, "http_requests_in_flight", nil)
	totalBefore := metricValue(t, "http_requests_total", series)

	for range 3 {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusInternalServerError, w.Code)
	}

	assert.InDelta(t, before, metricValue(t, "http_requests_in_flight", nil), 0,
		"in-flight gauge must return to its starting value")
	assert.InDelta(t, totalBefore+3, metricValue(t, "http_requests_total", series), 0,
		"each panicking request is counted as a 500")
}

func TestTracingMiddleware_PanicEndsSpanWithError(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	r := panicRouter(TracingMiddleware(nil), "/panic-trace")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panic-trace", nil))
	require.Equal(t, http.StatusInternalServerError, w.Code)

	spans := recorder.Ended()
	require.Len(t, spans, 1, "the span of a panicking request must still end")
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Contains(t, spans[0].Attributes(), attribute.Int("http.status_code", http.StatusInternalServerError))
}
