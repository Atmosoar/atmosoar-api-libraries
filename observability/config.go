// Package observability bootstraps Zap logging, Prometheus HTTP metrics,
// and OpenTelemetry tracing for Atmosoar Go services. It exposes a single
// init entry point plus an fx.Module for FX-based services, and a set of
// stable Gin middlewares that consumers wire into their own routers.
//
// The package is designed so that the multi-model-api (MMA) cutover is a
// drop-in import swap: metric names, label cardinality, tracing span names,
// and log format all match MMA's previous in-tree implementation
// byte-for-byte.
package observability

import (
	"os"
	"strings"
)

// AppStage mirrors config.AppStage values used by MMA.
type AppStage string

// Known AppStage values used by consumers of the library.
const (
	StageDevelopment AppStage = "development"
	StageLocal       AppStage = "local"
	StageProduction  AppStage = "production"
)

// Config controls how the observability package bootstraps logging,
// metrics, and tracing. Consumers may construct this directly or call
// ConfigFromEnv to build it from the same environment variables MMA uses
// today.
type Config struct {
	// Stage determines the Zap logger preset (development / production).
	Stage AppStage

	// ServiceName is the OTel service name. When TracingEnabled is true and
	// ServiceName is empty, a default of "dev-atmosoar-api" is used.
	ServiceName string

	// TracingEnabled gates the OTel exporter. When false, tracing is a no-op.
	TracingEnabled bool

	// OTLPEndpoint is the host:port of the OTel collector. Defaults to
	// "localhost:4317" when empty. Any http:// or https:// prefix is stripped.
	OTLPEndpoint string

	// MetricsSkipPath is the request path the HTTP metrics middleware excludes
	// from self-instrumentation (prevents /metrics endpoint self-inflation).
	// Defaults to "/metrics" when empty.
	MetricsSkipPath string

	// TraceSkipPaths are request paths the tracing middleware excludes to
	// avoid noise in Tempo. When nil or empty, no paths are skipped.
	TraceSkipPaths []string
}

// ConfigFromEnv builds a Config from the same environment variables MMA
// used in its in-tree implementation: OTEL_TRACES_ENABLED, OTEL_SERVICE_NAME,
// OTEL_EXPORTER_OTLP_ENDPOINT. Consumers that use a different config source
// can construct Config directly.
//
// The caller must provide the app stage (e.g. from their own config loader)
// and the metrics / trace skip paths (they're service-specific).
func ConfigFromEnv(stage AppStage, metricsSkipPath string, traceSkipPaths []string) Config {
	enabled := strings.EqualFold(os.Getenv("OTEL_TRACES_ENABLED"), "true")
	return Config{
		Stage:           stage,
		ServiceName:     os.Getenv("OTEL_SERVICE_NAME"),
		TracingEnabled:  enabled,
		OTLPEndpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		MetricsSkipPath: metricsSkipPath,
		TraceSkipPaths:  traceSkipPaths,
	}
}
