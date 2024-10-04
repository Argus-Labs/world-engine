package main

import (
	"context"
	"errors"
	"os"
	"strconv"

	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

const serviceName = "nakama"

func initOtelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	tracerProvider, err := newTracerProvider(ctx)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, // W3C Trace Context format; https://www.w3.org/TR/trace-context/
		),
	)

	return
}

func newTracerProvider(ctx context.Context) (*trace.TracerProvider, error) {
	globalJaegerAddress := os.Getenv(EnvJaegerAddr)
	globalJaegerSampleRate := os.Getenv(EnvJaegerSampleRate)

	if globalJaegerAddress == "" {
		return nil, eris.Errorf("must specify a jaeger server via %s", EnvJaegerAddr)
	}

	var sampleRate float64
	parsedSampleRate, err := strconv.ParseFloat(globalJaegerSampleRate, 64)
	if err != nil {
		sampleRate = 0.6
	} else {
		sampleRate = parsedSampleRate
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(globalJaegerAddress), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	resource := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
		// attribute.String("custom-attribute", "attribute-value"),
	)

	provider := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(resource),
		trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(sampleRate))),
	)

	return provider, nil
}
