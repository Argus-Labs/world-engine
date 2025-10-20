package telemetry

import (
	"context"
	"os"
	"time"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
)

type Telemetry struct {
	Logger      zerolog.Logger
	Tracer      trace.Tracer
	serviceName string

	shutdown func(context.Context) error
}

func New(opts Options) (Telemetry, error) {
	config, err := loadConfig()
	if err != nil {
		return Telemetry{}, eris.Wrap(err, "failed to load otel config")
	}

	options := newDefaultOptions()
	config.applyToOptions(&options)
	options.apply(opts)
	if err := options.validate(); err != nil {
		return Telemetry{}, eris.Wrap(err, "invalid otel options")
	}

	ctx := context.Background()
	tracer, logger, shutdown, err := setupOpenTelemetry(ctx, config.Enabled, options)
	if err != nil {
		return Telemetry{}, eris.Wrap(err, "failed to setup telemetry")
	}

	return Telemetry{
		Logger:      logger,
		Tracer:      tracer,
		serviceName: options.ServiceName,
		shutdown:    shutdown,
	}, nil
}

// Shutdown gracefully shuts down the telemetry system.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t.shutdown != nil {
		return t.shutdown(ctx)
	}
	return nil
}

// GetLogger returns a component-specific logger.
func (t *Telemetry) GetLogger(component string) zerolog.Logger {
	return t.Logger.With().Str("component", t.serviceName+"."+component).Logger()
}

// GetLoggerWithTrace returns a component-specific logger enriched with trace context.
func (t *Telemetry) GetLoggerWithTrace(ctx context.Context, component string) zerolog.Logger {
	span := trace.SpanFromContext(ctx)

	logger := t.Logger.With().Str("component", t.serviceName+"."+component)

	if span.IsRecording() {
		spanCtx := span.SpanContext()
		logger = logger.
			Str("trace_id", spanCtx.TraceID().String()).
			Str("span_id", spanCtx.SpanID().String())
	}

	return logger.Logger()
}

func init() { //nolint:gochecknoinits // Its fine
	// Set up the global logger
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Create a console writer with timestamp
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	// Set the global logger
	log.Logger = zerolog.New(consoleWriter). //nolint:reassign // Its fine
							With().
							Timestamp().
							Caller().
							Logger()
}

// GetGlobalLogger returns a component-specific logger using the global console logger.
func GetGlobalLogger(component string) zerolog.Logger {
	return log.With().Str("component", component).Logger()
}

// SetGlobalLogLevel sets the global log level for console logging.
// Deprecated: Use proper telemetry configuration instead.
func SetGlobalLogLevel(level string) {
	switch level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}
