package telemetry

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	otelTrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// setupOpenTelemetry sets up OpenTelemetry for the service.
// It returns a tracer, logger, and shutdown function.
func setupOpenTelemetry(
	ctx context.Context,
	enabled bool,
	opts Options,
) (otelTrace.Tracer, zerolog.Logger, func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error
	var err error

	shutdown := func(ctx context.Context) error {
		var shutdownErrs error
		for _, fn := range shutdownFuncs {
			shutdownErrs = errors.Join(shutdownErrs, fn(ctx))
		}
		shutdownFuncs = nil
		return shutdownErrs
	}

	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Setup logger first
	logger := newLogger(opts)

	// If telemetry is disabled, return noop tracer
	if !enabled {
		return noop.NewTracerProvider().Tracer(opts.ServiceName), logger, shutdown, nil
	}

	res, err := newResource(opts)
	if err != nil {
		handleErr(err)
		return noop.NewTracerProvider().Tracer(opts.ServiceName), logger, shutdown, err
	}

	propagator := newPropagator()
	otel.SetTextMapPropagator(propagator)

	tracerProvider, err := newTracerProvider(ctx, res, opts)
	if err != nil {
		handleErr(err)
		return noop.NewTracerProvider().Tracer(opts.ServiceName), logger, shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	tracer := tracerProvider.Tracer(opts.ServiceName)
	return tracer, logger, shutdown, err
}

func newResource(opts Options) (*resource.Resource, error) {
	return resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName(opts.ServiceName),
			semconv.ServiceVersion("dev"), // TODO: make configurable
		))
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider(ctx context.Context, res *resource.Resource, opts Options) (*trace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(opts.Endpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, eris.Wrap(err, "failed to create OTLP trace exporter")
	}

	var sampler trace.Sampler
	switch opts.TraceSampleRate {
	case 1.0:
		sampler = trace.AlwaysSample()
	case 0.0:
		sampler = trace.NeverSample()
	default:
		sampler = trace.ParentBased(trace.TraceIDRatioBased(opts.TraceSampleRate))
	}

	return trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
		trace.WithSampler(sampler),
	), nil
}

// newLogger creates a trace-aware logger with the specified format.
func newLogger(opts Options) zerolog.Logger {
	level, err := zerolog.ParseLevel(opts.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	var writer io.Writer
	switch opts.LogFormat {
	case LogFormatPretty:
		writer = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	case LogFormatJSON:
		writer = os.Stdout
	case LogFormatUndefined:
		assert.That(true, "unreachable")
	}

	return zerolog.New(writer).
		Level(level).
		With().
		Timestamp().
		Caller().
		Logger()
}
