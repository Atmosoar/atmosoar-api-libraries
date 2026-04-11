package observability

import (
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HTTP metric collectors. Names, help text, and label cardinality must
// stay byte-for-byte identical to MMA-168's original definitions so that
// existing dashboards and alerts continue to work after MMA's cutover.

// HTTPRequestsTotal counts HTTP requests processed, labelled by method/path/status.
//
//nolint:gochecknoglobals // parity with MMA-168; these are package-level singletons by design.
var HTTPRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests processed.",
	},
	[]string{"method", "path", "status"},
)

// HTTPRequestDuration records HTTP request latency, labelled by method/path.
//
//nolint:gochecknoglobals // parity with MMA-168; these are package-level singletons by design.
var HTTPRequestDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Histogram of HTTP request latencies.",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"method", "path"},
)

// HTTPRequestsInFlight gauges the number of requests currently being processed.
//
//nolint:gochecknoglobals // parity with MMA-168; these are package-level singletons by design.
var HTTPRequestsInFlight = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Number of HTTP requests currently being processed.",
	},
)

//nolint:gochecknoglobals // idempotency guard for Register().
var registerOnce sync.Once

//nolint:errname,gochecknoglobals // not a sentinel error; stores the first-registration error for Once semantics.
var registerErr error

// RegisterDefaultCollectors registers the library-owned HTTP metric
// collectors with the default Prometheus registry. Safe to call more
// than once — subsequent calls are no-ops. Returns the error from the
// first registration attempt, if any.
func RegisterDefaultCollectors() error {
	registerOnce.Do(func() {
		if err := prometheus.Register(HTTPRequestsTotal); err != nil {
			registerErr = err
			return
		}
		if err := prometheus.Register(HTTPRequestDuration); err != nil {
			registerErr = err
			return
		}
		if err := prometheus.Register(HTTPRequestsInFlight); err != nil {
			registerErr = err
			return
		}
	})
	return registerErr
}

// RegisterCollector lets a consumer add a custom collector (e.g. MMA's
// gRPC client metrics) to the default Prometheus registry. Useful when
// a service emits metrics the shared library doesn't own.
//
// Returns the same error prometheus.Register would return (e.g. on
// duplicate registration).
func RegisterCollector(c prometheus.Collector) error {
	return prometheus.Register(c)
}

// MetricsHandler returns the HTTP handler for the Prometheus scrape
// endpoint. Consumers wire this into their own router at whatever path
// they prefer (e.g. "/metrics", "/mma/metrics").
func MetricsHandler() gin.HandlerFunc {
	return gin.WrapH(promhttp.Handler())
}

// HTTPMetricsMiddleware returns a Gin middleware that records HTTP request
// counts, durations, and in-flight counts against the library collectors.
// The skipPath argument is the path to exclude from metric recording (the
// metrics scrape endpoint itself, to prevent self-inflation). When empty
// it defaults to "/metrics".
func HTTPMetricsMiddleware(skipPath string) gin.HandlerFunc {
	if skipPath == "" {
		skipPath = "/metrics"
	}
	return func(c *gin.Context) {
		if c.Request.URL.Path == skipPath {
			c.Next()
			return
		}

		HTTPRequestsInFlight.Inc()
		start := time.Now()

		c.Next()

		HTTPRequestsInFlight.Dec()
		elapsed := time.Since(start).Seconds()

		// Use Gin route template for low-cardinality labels; fall back to
		// /unknown for requests that didn't match any registered route.
		path := c.FullPath()
		if path == "" {
			path = "/unknown"
		}

		method := c.Request.Method
		status := strconv.Itoa(c.Writer.Status())

		HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		HTTPRequestDuration.WithLabelValues(method, path).Observe(elapsed)
	}
}
