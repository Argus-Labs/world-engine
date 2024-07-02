package telemetry

import (
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	ddotel "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
)

type Manager struct {
	tracerShutdownFunc   func() error
	profilerShutdownFunc func()
	tracerProvider       *ddotel.TracerProvider
}

func New(enableTrace bool, enableProfiler bool) (*Manager, error) {
	tm := Manager{
		tracerShutdownFunc: nil,
		tracerProvider:     nil,
	}

	// Set up propagator
	tm.setupPropagator()

	// Set up trace provider used for creating spans
	if enableTrace {
		tm.setupTrace()
	}

	// Set up profiler
	if enableProfiler {
		if err := tm.setupProfiler(); err != nil {
			return nil, errors.Join(err, tm.Shutdown())
		}
	}

	return &tm, nil
}

// Shutdown calls cleanup functions registered in the telemetry manager.
// Each registered cleanup will be invoked once and the errors from the calls are joined.
func (tm *Manager) Shutdown() error {
	if tm.tracerShutdownFunc != nil {
		err := tm.tracerShutdownFunc()
		return err
	}

	if tm.profilerShutdownFunc != nil {
		tm.profilerShutdownFunc()
	}

	return nil
}

func (tm *Manager) setupPropagator() {
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(prop)
}

func (tm *Manager) setupTrace() {
	tm.tracerProvider = ddotel.NewTracerProvider(tracer.WithRuntimeMetrics())
	tm.tracerShutdownFunc = tm.tracerProvider.Shutdown
	otel.SetTracerProvider(tm.tracerProvider)
}

func (tm *Manager) setupProfiler() error {
	err := profiler.Start(
		profiler.WithProfileTypes(
			profiler.CPUProfile,
			profiler.HeapProfile,
			// The profiles below are disabled by default to keep overhead
			// low, but can be enabled as needed.
			// profiler.BlockProfile,
			// profiler.MutexProfile,
			// profiler.GoroutineProfile,
		),
	)
	if err != nil {
		return err
	}

	tm.profilerShutdownFunc = profiler.Stop

	return nil
}
