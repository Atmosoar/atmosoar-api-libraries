package observability

import (
	"bytes"
	"fmt"
	"log"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Logger construction ports the original multi-model-api `logger` package
// verbatim. The Development preset is used for Development and Local stages;
// the Production preset is used for Production (and is the fallback for
// unknown stages).

var (
	bootBuf    bytes.Buffer
	bootLogger = log.New(&bootBuf, "observability: ", log.Lshortfile)
)

// NewLogger creates a SugaredLogger for the given stage. The caller owns
// the returned logger's lifetime; calling Sync() on shutdown is recommended.
func NewLogger(cfg Config) (*zap.SugaredLogger, error) {
	var baseLogger *zap.Logger
	var err error

	switch cfg.Stage {
	case StageDevelopment, StageLocal:
		baseLogger, err = zap.NewDevelopment()
	case StageProduction:
		baseLogger, err = zap.NewProduction()
	default:
		baseLogger, err = zap.NewProduction()
	}

	if err != nil {
		bootLogger.Panicln("can't initialize zap logger", "error", err)
		return nil, err
	}

	logger := baseLogger.Sugar()

	// Callers that want clean shutdown should call Sync() themselves when
	// their process is shutting down. The defer-on-construct pattern from
	// the original logger package is a no-op (it runs before the logger is
	// ever used), so we drop it here.

	return logger, nil
}

// NamedLogger returns an fx.Option that provides a name-tagged SugaredLogger
// derived from the base logger. Consumers can request it with
// `fx.In` + the matching `name:"..."` tag.
func NamedLogger(name string) fx.Option {
	return fx.Provide(
		fx.Annotate(
			func(base *zap.SugaredLogger) *zap.SugaredLogger {
				return base.Named(name)
			},
			fx.ResultTags(fmt.Sprintf(`name:"%s"`, name)),
		),
	)
}
