package telemetry

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// setupOpenTelemetry sets up OpenTelemetry for the service. It installs the global tracer
// provider and propagator that trace.New relies on, and returns the logger and a shutdown
// function. The globals are process-wide, so a process owns exactly one Telemetry: a second
// instance would take over the first one's spans, and shutting either down stops both.
func setupOpenTelemetry(
	ctx context.Context,
	opts Options,
) (zerolog.Logger, func(context.Context) error, error) {
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

	// An empty endpoint disables tracing: the global provider stays the SDK default no-op.
	if opts.Endpoint == "" {
		return logger, shutdown, nil
	}

	res, err := newResource(opts)
	if err != nil {
		handleErr(err)
		return logger, shutdown, err
	}

	propagator := newPropagator()
	otel.SetTextMapPropagator(propagator)

	// Route exporter failures through the service logger instead of OTel's own stderr logger.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Warn().Err(err).Msg("opentelemetry export failed")
	}))

	tracerProvider, err := newTracerProvider(ctx, res, opts)
	if err != nil {
		handleErr(err)
		return logger, shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	return logger, shutdown, err
}

func newResource(opts Options) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{semconv.ServiceName(opts.ServiceName)}
	if v := resolveServiceVersion(); v != "" {
		attrs = append(attrs, semconv.ServiceVersion(v))
	}
	return resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...))
}

// resolveServiceVersion picks the value for the service.version resource attribute.
// OTEL_SERVICE_VERSION wins; otherwise the binary's main-module version, then
// vcs.revision from build info. Empty result = attribute omitted.
func resolveServiceVersion() string {
	if v := os.Getenv("OTEL_SERVICE_VERSION"); v != "" {
		return v
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	// "(devel)" is what the Go toolchain emits for an untagged main module;
	// treat it as no-version and fall through to vcs.revision.
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return ""
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// exporterEndpointOptions maps a resolved endpoint onto exporter options. A URL endpoint is handed to the
// exporter whole so its scheme selects the transport; a bare host:port is dialed with TLS unless insecure.
func exporterEndpointOptions(endpoint string, insecure bool) []otlptracegrpc.Option {
	if strings.Contains(endpoint, "://") {
		return []otlptracegrpc.Option{otlptracegrpc.WithEndpointURL(endpoint)}
	}
	options := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
	if insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}
	return options
}

func newTracerProvider(ctx context.Context, res *resource.Resource, opts Options) (*trace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx, exporterEndpointOptions(opts.Endpoint, opts.Insecure)...)
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
