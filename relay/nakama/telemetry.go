package main

import (
	"context"
	"errors"
	"os"
	"strconv"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

const serviceName = "nakama"

func initOtelSDK(ctx context.Context, logger runtime.Logger) (func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error
	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	enableTrace := false
	globalTraceEnabled, err := strconv.ParseBool(os.Getenv(EnvTraceEnabled))
	if err == nil {
		enableTrace = globalTraceEnabled
	}

	if enableTrace {
		tracerProvider, err := newTracerProvider(ctx, logger)
		if err != nil {
			return nil, errors.Join(err, shutdown(ctx))
		}
		shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
		otel.SetTracerProvider(tracerProvider)
	}

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, // W3C Trace Context format; https://www.w3.org/TR/trace-context/
			propagation.Baggage{},
		),
	)

	return shutdown, nil
}

func newTracerProvider(ctx context.Context, logger runtime.Logger) (*trace.TracerProvider, error) {
	globalJaegerAddress := os.Getenv(EnvJaegerAddr)
	globalJaegerSampleRate := os.Getenv(EnvJaegerSampleRate)

	if globalJaegerAddress == "" {
		return nil, eris.Errorf("must specify a jaeger server via %s", EnvJaegerAddr)
	}

	var sampleRate float64
	parsedSampleRate, err := strconv.ParseFloat(globalJaegerSampleRate, 64)
	if err != nil {
		logger.Info("Invalid sample rate %s, defaulting to 0.6", globalJaegerSampleRate)
		sampleRate = 0.6
	} else {
		sampleRate = parsedSampleRate
	}

	if sampleRate < 0 || sampleRate > 1 {
		return nil, eris.Errorf("trace sample rate must be between 0 and 1, got %f", sampleRate)
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(globalJaegerAddress), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, eris.Wrap(err, "failed to create otlp exporter")
	}

	resource := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
	)

	provider := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(resource),
		trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(sampleRate))),
	)

	return provider, nil
}
