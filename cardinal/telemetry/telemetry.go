package telemetry

import (
	"context"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

type Manager struct {
	namespace          string
	tracerShutdownFunc func() error
	tracerProvider     *trace.TracerProvider
}

func New(enableTrace bool, namespace string) (*Manager, error) {
	ctx := context.Background()

	tm := Manager{
		namespace:          namespace,
		tracerShutdownFunc: nil,
		tracerProvider:     nil,
	}

	// Set up propagator
	exporter, err := tm.setupExporter(ctx)
	if err != nil {
		return nil, err
	}

	// Set up trace provider used for creating spans
	if enableTrace {
		err := tm.setupTrace(exporter)
		if err != nil {
			return nil, err
		}
	}

	return &tm, nil
}

// Shutdown calls cleanup functions registered in the telemetry manager.
// Each registered cleanup will be invoked once and the errors from the calls are joined.
func (tm *Manager) Shutdown() error {
	log.Debug().Msg("Shutting down telemetry")

	if tm.tracerShutdownFunc != nil {
		err := tm.tracerShutdownFunc()
		return err
	}

	log.Debug().Msg("Successfully shutdown telemetry")
	return nil
}

func (tm *Manager) setupExporter(ctx context.Context) (trace.SpanExporter, error) {
	return otlptracegrpc.New(ctx)
}

func (tm *Manager) setupTrace(exporter trace.SpanExporter) error {
	// Ensure default SDK resources and the required service name are set.
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(tm.namespace+"-cardinal"),
		),
	)
	if err != nil {
		return err
	}

	tm.tracerProvider = trace.NewTracerProvider(trace.WithResource(r), trace.WithBatcher(exporter))
	otel.SetTracerProvider(tm.tracerProvider)

	return nil
}
