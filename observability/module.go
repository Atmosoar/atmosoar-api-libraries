package observability

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Module is the FX module for the observability package. It provides:
//
//   - *zap.SugaredLogger — the root logger, derived from the injected Config
//   - ShutdownFunc — the tracing shutdown closure, registered as an fx.Lifecycle hook
//
// Consumers are expected to provide a Config value via their own fx.Provide.
// The Module also calls RegisterDefaultCollectors so the HTTP metric
// collectors are available for scraping as soon as the app starts.
//
// Example wiring in a consumer's main.go:
//
//	fx.New(
//	    fx.Provide(func() observability.Config {
//	        return observability.ConfigFromEnv(
//	            observability.StageProduction,
//	            "/mma/metrics",
//	            []string{"/mma/metrics", "/health"},
//	        )
//	    }),
//	    observability.Module,
//	    // ... your other modules
//	).Run()
//
// Consumers then request `*zap.SugaredLogger` via fx.In in their constructors.
var Module = fx.Module(
	"observability",
	fx.Provide(NewLogger),
	fx.Invoke(registerDefaultCollectorsHook),
	fx.Invoke(setupTracing),
)

// registerDefaultCollectorsHook registers the library's HTTP collectors with
// the default Prometheus registry. This is a fire-and-forget FX hook; the
// error is logged via the injected logger but does not fail app startup.
func registerDefaultCollectorsHook(logger *zap.SugaredLogger) {
	if err := RegisterDefaultCollectors(); err != nil {
		logger.Warnw("observability: default collector registration returned error",
			"error", err)
	}
}

// setupTracing initializes the OTel tracer from Config and registers a
// graceful shutdown hook on the FX lifecycle.
func setupTracing(lc fx.Lifecycle, cfg Config, logger *zap.SugaredLogger) {
	shutdown := InitTracing(cfg, logger)
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return shutdown(ctx)
		},
	})
}
