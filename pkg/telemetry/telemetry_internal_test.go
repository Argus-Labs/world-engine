package telemetry

import (
	"context"
	"errors"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	sentrygo "github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	collmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	colltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

// initSentryRecorder initializes the global Sentry hub with a BeforeSend hook that records every
// error passed to sentrygo.CaptureException, then drops the event (returns nil) so no network I/O
// occurs. It restores the previous hub client on test completion. The returned slice is shared
// across the test; clear it between subtests.
func initSentryRecorder(t *testing.T) *[]error {
	t.Helper()
	var captured []error
	prevClient := sentrygo.CurrentHub().Client()
	err := sentrygo.Init(sentrygo.ClientOptions{
		DisableTelemetryBuffer: true,
		BeforeSend: func(_ *sentrygo.Event, hint *sentrygo.EventHint) *sentrygo.Event {
			if hint != nil && hint.OriginalException != nil {
				captured = append(captured, hint.OriginalException)
			}
			return nil
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		sentrygo.Flush(2 * time.Second)
		sentrygo.CurrentHub().BindClient(prevClient)
	})
	return &captured
}

// errRealShutdown is a sentinel used to distinguish real shutdown failures from benign
// cancellation-shaped ones in the guard tests below.
var errRealShutdown = errors.New("real shutdown failure")

// TestMaxCommandLinksResolvesFromEnv verifies the span link limit threaded out of provider setup
// mirrors OTEL_SPAN_LINK_COUNT_LIMIT (default 128). Cardinal caps tick-span links at this value so
// the cap and the tracer provider share one source of truth: no link is built only to be dropped
// by the SDK. An empty exporter endpoint disables the real exporter but still resolves the limit
// from the same env source, so a caller that later installs its own provider reading the same env
// stays in sync — this is exactly the path the cardinal trace tests take.
func TestMaxCommandLinksResolvesFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     int
	}{
		{name: "empty falls back to default 128", envValue: "", want: 128},
		{name: "below default", envValue: "64", want: 64},
		{name: "above default", envValue: "256", want: 256},
		{name: "invalid value falls back to default", envValue: "not-an-int", want: 128},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_SPAN_LINK_COUNT_LIMIT", tt.envValue)
			// Disable the real exporter so setupOpenTelemetry takes the empty-endpoint path, which
			// still resolves the link limit from the same env source.
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
			t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
			t.Setenv("LOG_LEVEL", "info")
			t.Setenv("LOG_FORMAT", "json")

			tel, err := New(Options{ServiceName: "test"})
			require.NoError(t, err)
			require.Equal(t, tt.want, tel.MaxCommandLinks())
		})
	}
}

// fakeCollector is an OTLP gRPC metrics service that records the metrics it receives.
type fakeCollector struct {
	collmetricpb.UnimplementedMetricsServiceServer

	mu      sync.Mutex
	metrics []string // "<service.name>/<metric name>" per received data point series
}

func (c *fakeCollector) Export(
	_ context.Context, req *collmetricpb.ExportMetricsServiceRequest,
) (*collmetricpb.ExportMetricsServiceResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rm := range req.GetResourceMetrics() {
		service := ""
		for _, attr := range rm.GetResource().GetAttributes() {
			if attr.GetKey() == "service.name" {
				service = attr.GetValue().GetStringValue()
			}
		}
		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				c.metrics = append(c.metrics, service+"/"+m.GetName())
			}
		}
	}
	return &collmetricpb.ExportMetricsServiceResponse{}, nil
}

func (c *fakeCollector) received() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.metrics)
}

// stalledTraceService accepts span exports and never answers them, like a collector that hangs.
type stalledTraceService struct {
	colltracepb.UnimplementedTraceServiceServer
}

func (stalledTraceService) Export(
	ctx context.Context, _ *colltracepb.ExportTraceServiceRequest,
) (*colltracepb.ExportTraceServiceResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// startFakeCollector serves a fakeCollector and the given trace service on a loopback port, and
// returns the fakeCollector with its host:port.
func startFakeCollector(t *testing.T, traces colltracepb.TraceServiceServer) (*fakeCollector, string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	collector := &fakeCollector{}
	server := grpc.NewServer()
	collmetricpb.RegisterMetricsServiceServer(server, collector)
	colltracepb.RegisterTraceServiceServer(server, traces)
	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.Stop)
	return collector, lis.Addr().String()
}

// restoreGlobalProviders puts back the global providers New replaces once the test ends. It
// restores only what changed: re-setting a provider to itself logs a warning.
func restoreGlobalProviders(t *testing.T) {
	t.Helper()
	prevMeters, prevTracers := otel.GetMeterProvider(), otel.GetTracerProvider()
	t.Cleanup(func() {
		if otel.GetMeterProvider() != prevMeters {
			otel.SetMeterProvider(prevMeters)
		}
		if otel.GetTracerProvider() != prevTracers {
			otel.SetTracerProvider(prevTracers)
		}
	})
}

// TestNewExportsMetricsOnlyToMetricsEndpoint checks that a metric recorded through the global meter
// reaches the collector named by OTEL_EXPORTER_OTLP_METRICS_ENDPOINT, and that a collector named only
// by OTEL_EXPORTER_OTLP_ENDPOINT (the cardinal operator's traces-only receiver) never gets metrics.
func TestNewExportsMetricsOnlyToMetricsEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		envFor     func(addr string) map[string]string
		wantMetric []string
	}{
		{
			name: "metrics endpoint receives metrics",
			envFor: func(addr string) map[string]string {
				return map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": addr}
			},
			wantMetric: []string{"test/test.requests"},
		},
		{
			name: "generic endpoint receives no metrics",
			envFor: func(addr string) map[string]string {
				return map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": addr, "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": ""}
			},
			wantMetric: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector, addr := startFakeCollector(t, colltracepb.UnimplementedTraceServiceServer{})
			for key, value := range tt.envFor(addr) {
				t.Setenv(key, value)
			}
			t.Setenv("LOG_LEVEL", "info")
			t.Setenv("LOG_FORMAT", "json")
			restoreGlobalProviders(t)

			tel, err := New(Options{ServiceName: "test"})
			require.NoError(t, err)
			counter, err := otel.Meter("test").Int64Counter("test.requests")
			require.NoError(t, err)
			counter.Add(context.Background(), 1)
			require.NoError(t, tel.Shutdown(context.Background())) // Flushes the final export.

			require.Equal(t, tt.wantMetric, collector.received())
		})
	}
}

// TestShutdownFlushesMetricsPastStalledTraceCollector checks that a trace collector that never
// answers cannot use up the shutdown deadline before the meter provider's final flush.
func TestShutdownFlushesMetricsPastStalledTraceCollector(t *testing.T) {
	collector, addr := startFakeCollector(t, stalledTraceService{})
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", addr)
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", addr)
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("LOG_FORMAT", "json")
	restoreGlobalProviders(t)

	tel, err := New(Options{ServiceName: "test"})
	require.NoError(t, err)
	_, span := otel.Tracer("test").Start(context.Background(), "stalls")
	span.End()
	counter, err := otel.Meter("test").Int64Counter("test.requests")
	require.NoError(t, err)
	counter.Add(context.Background(), 1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.ErrorIs(t, tel.Shutdown(ctx), context.DeadlineExceeded) // The span flush never completes.
	require.Equal(t, []string{"test/test.requests"}, collector.received())
}

// TestShutdownCancellationGuard verifies that Telemetry.Shutdown reports only real shutdown
// failures to Sentry and suppresses benign [context.Canceled] / [context.DeadlineExceeded] errors —
// including when they are buried inside an [errors.Join] tree, which is the shape setup.go's
// shutdown closure always produces ([errors.Join](errs...)) and which eris.Is (v0.5.4) cannot
// traverse. The guard must use stdlib [errors.Is] so the sentinel checks match through join trees.
func TestShutdownCancellationGuard(t *testing.T) {
	captured := initSentryRecorder(t)

	posthogClient, err := posthog.New(posthog.Options{})
	require.NoError(t, err)

	tests := []struct {
		name         string
		shutdownErr  error
		wantCaptured bool // whether CaptureException should fire
		checkErr     func(t *testing.T, err error)
	}{
		// Benign cancellation/deadline errors — must NOT be captured, regardless of nesting.
		{
			name:        "bare context.DeadlineExceeded",
			shutdownErr: context.DeadlineExceeded,
			checkErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, context.DeadlineExceeded) },
		},
		{
			name:        "bare context.Canceled",
			shutdownErr: context.Canceled,
			checkErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, context.Canceled) },
		},
		{
			name:        "single-element join of DeadlineExceeded",
			shutdownErr: errors.Join(context.DeadlineExceeded),
			checkErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, context.DeadlineExceeded) },
		},
		{
			name:        "single-element join of Canceled",
			shutdownErr: errors.Join(context.Canceled),
			checkErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, context.Canceled) },
		},
		{
			name:        "doubly-joined DeadlineExceeded mimicking production chain",
			shutdownErr: errors.Join(context.DeadlineExceeded, errors.Join(context.DeadlineExceeded, nil)),
			checkErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, context.DeadlineExceeded) },
		},
		{
			name:        "join containing Canceled alongside another error",
			shutdownErr: errors.Join(context.Canceled, errRealShutdown),
			checkErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, context.Canceled) },
		},

		// Real shutdown failures — MUST be captured.
		{
			name:         "bare real error",
			shutdownErr:  errRealShutdown,
			wantCaptured: true,
			checkErr:     func(t *testing.T, err error) { require.ErrorIs(t, err, errRealShutdown) },
		},
		{
			name:         "single-element join of real error",
			shutdownErr:  errors.Join(errRealShutdown),
			wantCaptured: true,
			checkErr:     func(t *testing.T, err error) { require.ErrorIs(t, err, errRealShutdown) },
		},
		{
			name:         "multi-element join of real errors",
			shutdownErr:  errors.Join(errRealShutdown, errors.New("another failure")),
			wantCaptured: true,
			checkErr:     func(t *testing.T, err error) { require.ErrorIs(t, err, errRealShutdown) },
		},

		// No error — nothing to capture.
		{
			name:        "nil shutdown error",
			shutdownErr: nil,
			checkErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*captured = nil

			tel := Telemetry{
				posthog:  posthogClient,
				shutdown: func(context.Context) error { return tt.shutdownErr },
			}

			err := tel.Shutdown(context.Background())
			tt.checkErr(t, err)

			if tt.wantCaptured {
				require.Len(t, *captured, 1, "expected the shutdown error to be captured to Sentry")
			} else {
				require.Empty(t, *captured, "expected no Sentry capture for a benign cancellation error")
			}
		})
	}
}
