package runtimeconfig

import (
	"context"

	"go.uber.org/fx"
)

// Module is the FX module for runtime configuration. It provides a *Manager built
// from a *Registry and a Store the consumer supplies via their own fx.Provide, and
// registers an OnStart hook that loads persisted overrides at boot.
//
// Example wiring in a consumer:
//
//	fx.New(
//	    fx.Provide(buildRegistry),                 // *runtimeconfig.Registry
//	    fx.Provide(buildStore),                    // runtimeconfig.Store
//	    observability.Module,                      // *zap.SugaredLogger
//	    runtimeconfig.Module,                      // *runtimeconfig.Manager
//	    admin.Module,                              // admin REST surface over Manager
//	    // ... service modules read via manager.Int/Float/Bool/String/Duration
//	)
var Module = fx.Module(
	"runtimeconfig",
	fx.Provide(NewManager),
	fx.Invoke(registerLifecycle),
)

func registerLifecycle(lc fx.Lifecycle, m *Manager) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return m.LoadAtBoot(ctx)
		},
	})
}
