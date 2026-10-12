package telemetry

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// setupOpenTelemetry sets up OpenTelemetry for the service. It installs the global tracer
// provider and propagator that trace.New relies on, and the global meter provider when a metrics
// endpoint is set. It returns the logger, the resolved span link limit, and a shutdown function.
// The globals are process-wide, so a process owns exactly one Telemetry: a second instance would
// take over the first one's spans and metrics, and shutting either down stops both.
//
// The resolved link limit mirrors what the tracer provider applies (OTEL_SPAN_LINK_COUNT_LIMIT,
// default 128): it is threaded out so callers that pre-cap links (cardinal's tick span) can match
// the provider's actual limit and never build links the SDK would drop. When tracing is disabled
// (empty endpoint) no provider is constructed, but the limit is still resolved from the same env
// source so a caller that later installs its own provider reading the same env stays in sync.
func setupOpenTelemetry(
	ctx context.Context,
	opts Options,
) (zerolog.Logger, int, func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error
	var err error

	// Providers shut down concurrently, so an exporter stalled on an unreachable collector cannot
	// spend the deadline the others need for their final flush.
	shutdown := func(ctx context.Context) error {
		errs := make([]error, len(shutdownFuncs))
		var wg sync.WaitGroup
		for i, fn := range shutdownFuncs {
			wg.Go(func() { errs[i] = fn(ctx) })
		}
		wg.Wait()
		shutdownFuncs = nil
		return errors.Join(errs...)
	}

	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Setup logger first
	logger := newLogger(opts)

	// Resolve the span link limit the same way the SDK does (OTEL_SPAN_LINK_COUNT_LIMIT, default
	// 128). Threaded out so link cappers share the provider's actual limit.
	linkLimit := trace.NewSpanLimits().LinkCountLimit

	// An empty endpoint disables its signal: that global provider stays the SDK default no-op.
	if opts.Endpoint == "" && opts.MetricsEndpoint == "" {
		return logger, linkLimit, shutdown, nil
	}

	res, err := newResource(opts)
	if err != nil {
		handleErr(err)
		return logger, linkLimit, shutdown, err
	}

	// Route exporter failures through the service logger instead of OTel's own stderr logger.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Warn().Err(err).Msg("opentelemetry export failed")
	}))

	if opts.Endpoint != "" {
		otel.SetTextMapPropagator(newPropagator())

		tracerProvider, providerLinkLimit, err := newTracerProvider(ctx, res, opts)
		if err != nil {
			handleErr(err)
			return logger, linkLimit, shutdown, err
		}
		shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
		otel.SetTracerProvider(tracerProvider)
		linkLimit = providerLinkLimit
	}

	if opts.MetricsEndpoint != "" {
		meterProvider, err := newMeterProvider(ctx, res, opts)
		if err != nil {
			handleErr(err)
			return logger, linkLimit, shutdown, err
		}
		shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
		otel.SetMeterProvider(meterProvider)
	}

	return logger, linkLimit, shutdown, err
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

// metricExporterEndpointOptions is exporterEndpointOptions for the metric exporter.
func metricExporterEndpointOptions(endpoint string, insecure bool) []otlpmetricgrpc.Option {
	if strings.Contains(endpoint, "://") {
		return []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpointURL(endpoint)}
	}
	options := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(endpoint)}
	if insecure {
		options = append(options, otlpmetricgrpc.WithInsecure())
	}
	return options
}

// newMeterProvider builds the SDK MeterProvider that pushes to the metrics endpoint every
// OTEL_METRIC_EXPORT_INTERVAL (default 60s), and once more on shutdown.
func newMeterProvider(ctx context.Context, res *resource.Resource, opts Options) (*metric.MeterProvider, error) {
	exporter, err := otlpmetricgrpc.New(
		ctx,
		metricExporterEndpointOptions(opts.MetricsEndpoint, opts.MetricsInsecure)...)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create OTLP metric exporter")
	}
	return metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exporter)),
		metric.WithResource(res),
	), nil
}

// newTracerProvider builds the SDK TracerProvider for the resolved config and returns it together
// with the resolved span link limit. The limit is captured here — at the point the provider is
// constructed, which is where the SDK reads OTEL_SPAN_LINK_COUNT_LIMIT — so the returned value is
// the exact LinkCountLimit the provider will enforce. Callers thread it to link cappers so the
// cap and the provider share one source of truth.
func newTracerProvider(ctx context.Context, res *resource.Resource, opts Options) (*trace.TracerProvider, int, error) {
	exporter, err := otlptracegrpc.New(ctx, exporterEndpointOptions(opts.Endpoint, opts.Insecure)...)
	if err != nil {
		return nil, 0, eris.Wrap(err, "failed to create OTLP trace exporter")
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

	// Read the SDK's link limit at construction time (the same value NewTracerProvider applies
	// when no WithSpanLimits is passed). Threading this out keeps a future explicit WithSpanLimits
	// authoritative: the cap will track whatever the provider actually enforces.
	linkLimit := trace.NewSpanLimits().LinkCountLimit

	return trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
		trace.WithSampler(sampler),
	), linkLimit, nil
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
