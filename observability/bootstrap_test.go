package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("OTEL_TRACES_ENABLED", "TRUE")
	t.Setenv("OTEL_SERVICE_NAME", "radar")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://otel:4317")
	cfg := ConfigFromEnv(StageProduction, "/m", []string{"/health"})
	if !cfg.TracingEnabled || cfg.ServiceName != "radar" || cfg.OTLPEndpoint != "http://otel:4317" ||
		cfg.Stage != StageProduction || cfg.MetricsSkipPath != "/m" || len(cfg.TraceSkipPaths) != 1 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	t.Setenv("OTEL_TRACES_ENABLED", "1")
	if ConfigFromEnv(StageLocal, "", nil).TracingEnabled {
		t.Fatal(`only "true" enables tracing`)
	}
}

func TestNewLoggerPresetPerStage(t *testing.T) {
	for stage, debug := range map[AppStage]bool{
		StageDevelopment: true,
		StageLocal:       true,
		StageProduction:  false,
		AppStage("qa"):   false, // unknown stages fall back to production
	} {
		l, err := NewLogger(Config{Stage: stage})
		if err != nil {
			t.Fatalf("%s: %v", stage, err)
		}
		if got := l.Desugar().Core().Enabled(zapcore.DebugLevel); got != debug {
			t.Fatalf("%s: debug enabled = %v, want %v", stage, got, debug)
		}
	}
}

func TestInitTracingDisabledIsNoop(t *testing.T) {
	before := otel.GetTracerProvider()
	shutdown := InitTracing(Config{}, zap.NewNop().Sugar())
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if otel.GetTracerProvider() != before {
		t.Fatal("disabled tracing must not replace the global provider")
	}
}

func TestInitTracingEnabledWithUnreachableCollector(t *testing.T) {
	previous := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	// The gRPC exporter connects lazily, so an unreachable endpoint still
	// yields a provider; startup must not block or fail on it.
	shutdown := InitTracing(Config{TracingEnabled: true, OTLPEndpoint: "https://127.0.0.1:1"}, nil)
	if otel.GetTracerProvider() == previous {
		t.Fatal("enabled tracing must install a provider")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = shutdown(ctx) // flushing nothing to a dead collector may report an error; it must return
}

func TestRegisterDefaultCollectorsIsIdempotent(t *testing.T) {
	if err := RegisterDefaultCollectors(); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := RegisterDefaultCollectors(); err != nil {
		t.Fatalf("second call must be a no-op: %v", err)
	}
	c := prometheus.NewCounter(prometheus.CounterOpts{Name: "observability_test_custom_total", Help: "x"})
	if err := RegisterCollector(c); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { prometheus.Unregister(c) })
	if err := RegisterCollector(c); err == nil {
		t.Fatal("duplicate custom collector must be rejected")
	}
}

func TestMetricsMiddlewareLabelsAndSkipPath(t *testing.T) {
	_ = RegisterDefaultCollectors()
	r := gin.New()
	r.Use(HTTPMetricsMiddleware(""))
	r.GET("/metrics", MetricsHandler())
	r.GET("/items/:id", func(c *gin.Context) { c.Status(http.StatusTeapot) })

	before := counterValue(t, HTTPRequestsTotal.WithLabelValues("GET", "/items/:id", "418"))
	beforeUnknown := counterValue(t, HTTPRequestsTotal.WithLabelValues("GET", "/unknown", "404"))
	beforeSkip := counterValue(t, HTTPRequestsTotal.WithLabelValues("GET", "/metrics", "200"))

	for _, p := range []string{"/items/42", "/no/such/route", "/metrics"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if p == "/metrics" && !strings.Contains(w.Body.String(), "http_requests_total") {
			t.Fatal("metrics handler must expose the library collectors")
		}
	}

	if got := counterValue(t, HTTPRequestsTotal.WithLabelValues("GET", "/items/:id", "418")); got != before+1 {
		t.Fatalf("route template label: got %v want %v", got, before+1)
	}
	if got := counterValue(t, HTTPRequestsTotal.WithLabelValues("GET", "/unknown", "404")); got != beforeUnknown+1 {
		t.Fatalf("unmatched route must count as /unknown: got %v", got)
	}
	if got := counterValue(t, HTTPRequestsTotal.WithLabelValues("GET", "/metrics", "200")); got != beforeSkip {
		t.Fatal("the scrape path must not count itself")
	}
}

func TestModuleProvidesLoggerAndNamedLogger(t *testing.T) {
	var named struct {
		fx.In
		Log *zap.SugaredLogger `name:"radar"`
	}
	var base *zap.SugaredLogger
	app := fxtest.New(t,
		fx.Supply(Config{Stage: StageProduction}),
		Module,
		NamedLogger("radar"),
		fx.Populate(&base),
		fx.Invoke(func(p struct {
			fx.In
			Log *zap.SugaredLogger `name:"radar"`
		}) {
			named.Log = p.Log
		}),
	)
	app.RequireStart()
	app.RequireStop()
	if base == nil || named.Log == nil {
		t.Fatal("module must provide the base and the named logger")
	}
}

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatal(err)
	}
	return m.GetCounter().GetValue()
}
