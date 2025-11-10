package telemetry

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	"github.com/argus-labs/world-engine/pkg/telemetry/sentry"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
)

type Telemetry struct {
	Logger      zerolog.Logger
	Tracer      trace.Tracer
	posthog     *posthog.Client
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

	err = sentry.New(options.SentryOptions)
	if err != nil {
		return Telemetry{}, eris.Wrap(err, "failed to setup sentry")
	}

	posthog, err := posthog.New(options.PosthogOptions)
	if err != nil {
		return Telemetry{}, eris.Wrap(err, "failed to setup posthog")
	}

	return Telemetry{
		Logger:      logger,
		Tracer:      tracer,
		posthog:     posthog,
		serviceName: options.ServiceName,
		shutdown:    shutdown,
	}, nil
}

// Shutdown gracefully shuts down the telemetry system.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	var outErr error

	if t.shutdown != nil {
		err := t.shutdown(ctx)
		if err != nil && !eris.Is(err, context.Canceled) && !eris.Is(err, context.DeadlineExceeded) {
			t.CaptureException(ctx, err)
		}
		outErr = errors.Join(outErr, eris.Wrap(err, "otel shutdown"))
	}

	if err := t.posthog.Shutdown(); err != nil {
		t.CaptureException(ctx, err)
		outErr = errors.Join(outErr, eris.Wrap(err, "posthog shutdown"))
	}

	sentry.Shutdown(ctx, 5*time.Second)
	return outErr
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

// CaptureException captures an exception and sends it to Sentry.
func (t *Telemetry) CaptureException(ctx context.Context, err error) {
	sentry.CaptureException(ctx, err)
}

// RecoverAndFlush recovers from a panic and flushes buffered events to Sentry.
func (t *Telemetry) RecoverAndFlush(repanic bool) {
	sentry.RecoverAndFlush(repanic)
}

// CaptureEvent captures an event and sends it to PostHog.
func (t *Telemetry) CaptureEvent(ctx context.Context, event string, properties map[string]any) {
	err := t.posthog.Capture(ctx, event, properties)
	if err != nil {
		// If we fail to capture the event, we still want to capture the exception.
		// We can track the error in Sentry but not cause user to fail the application.
		t.CaptureException(ctx, err)
	}
}

func init() { //nolint:gochecknoinits // Its fine
	config, err := loadConfig()
	if err != nil {
		// Default to info level and JSON format.
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		log.Logger = zerolog.New(os.Stdout). //nolint:reassign // Its fine
							With().
							Timestamp().
							Caller().
							Logger()
		return
	}

	opts := newDefaultOptions()
	config.applyToOptions(&opts)

	level, err := zerolog.ParseLevel(opts.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	var writer io.Writer = os.Stdout // Default to JSON
	if opts.LogFormat == LogFormatPretty {
		writer = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	log.Logger = zerolog.New(writer). //nolint:reassign // Its fine
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
