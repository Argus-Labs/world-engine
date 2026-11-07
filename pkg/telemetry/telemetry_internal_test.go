package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	sentrygo "github.com/getsentry/sentry-go"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	collmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	colltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

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

// stalledMetricsService accepts metric exports and never answers them, like a metrics
// collector that hangs. The OTLP gRPC exporter surfaces this as a [codes.DeadlineExceeded]
// status joined with [context.DeadlineExceeded] when the shutdown ctx expires.
type stalledMetricsService struct {
	collmetricpb.UnimplementedMetricsServiceServer
}

func (stalledMetricsService) Export(
	ctx context.Context, _ *collmetricpb.ExportMetricsServiceRequest,
) (*collmetricpb.ExportMetricsServiceResponse, error) {
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

// startStalledMetricsServer serves a metrics service that never answers (and a no-op trace
// service) on a loopback port, returning the host:port. Used to reproduce the pure-timeout
// shutdown that must stay out of Sentry.
func startStalledMetricsServer(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	collmetricpb.RegisterMetricsServiceServer(server, stalledMetricsService{})
	colltracepb.RegisterTraceServiceServer(server, colltracepb.UnimplementedTraceServiceServer{})
	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.Stop)
	return lis.Addr().String()
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

// errShutdownReal is the canonical real export failure used across the shared shutdown
// error-case table.
var errShutdownReal = errors.New("export failed: connection refused")

// shutdownErrorCases is the shared matrix of shutdown error trees exercised by both the
// Sentry integration test (TestShutdownSkipsSentryForJoinedCancellation) and the helper
// test (TestOnlyContextSentinels). sentinel reports whether onlyContextSentinels
// classifies the tree as a pure context cancellation (no Sentry event); the integration
// test inverts it into a capture count so its assertions stay distinct from the helper's
// boolean assertions.
//
// Mixed joins are the shape setup.go's concurrent shutdown naturally produces when one
// provider stalls on a canceled ctx while another fails with a real export error: the
// real error must still be captured even though a context sentinel is joined in, so
// every leaf must be checked.
var shutdownErrorCases = []struct {
	name     string
	err      error
	sentinel bool
}{
	{name: "bare Canceled", err: context.Canceled, sentinel: true},
	{name: "bare DeadlineExceeded", err: context.DeadlineExceeded, sentinel: true},
	{name: "join(Canceled)", err: errors.Join(context.Canceled), sentinel: true},
	{name: "join(DeadlineExceeded)", err: errors.Join(context.DeadlineExceeded), sentinel: true},
	{
		name:     "join(Canceled, DeadlineExceeded)",
		err:      errors.Join(context.Canceled, context.DeadlineExceeded),
		sentinel: true,
	},
	{name: "wrapped Canceled", err: fmt.Errorf("ctx: %w", context.Canceled), sentinel: true},
	{
		name:     "wrapped join(Canceled, DeadlineExceeded)",
		err:      fmt.Errorf("ctx: %w", errors.Join(context.Canceled, context.DeadlineExceeded)),
		sentinel: true,
	},

	{name: "bare real", err: errShutdownReal, sentinel: false},
	{name: "join(real)", err: errors.Join(errShutdownReal), sentinel: false},
	{name: "bare eris real", err: eris.New("export failed"), sentinel: false},
	{name: "join(Canceled, eris real)", err: errors.Join(context.Canceled, eris.New("export failed")), sentinel: false},

	{name: "join(Canceled, real)", err: errors.Join(context.Canceled, errShutdownReal), sentinel: false},
	{name: "join(real, Canceled)", err: errors.Join(errShutdownReal, context.Canceled), sentinel: false},
	{
		name:     "join(DeadlineExceeded, real)",
		err:      errors.Join(context.DeadlineExceeded, errShutdownReal),
		sentinel: false,
	},
	{
		name:     "join(real, DeadlineExceeded)",
		err:      errors.Join(errShutdownReal, context.DeadlineExceeded),
		sentinel: false,
	},
	{
		name:     "join(Canceled, DeadlineExceeded, real)",
		err:      errors.Join(context.Canceled, context.DeadlineExceeded, errShutdownReal),
		sentinel: false,
	},
	{
		name:     "fmt.Errorf wraps join(Canceled, real)",
		err:      fmt.Errorf("otel flush: %w", errors.Join(context.Canceled, errShutdownReal)),
		sentinel: false,
	},
	{
		name:     "fmt.Errorf wraps join(real, Canceled)",
		err:      fmt.Errorf("otel flush: %w", errors.Join(errShutdownReal, context.Canceled)),
		sentinel: false,
	},
}

// TestShutdownSkipsSentryForJoinedCancellation verifies the cancellation guard in Shutdown
// sees through [errors.Join], the shape setup.go's shutdown returns. Pure context-sentinel
// trees (bare, joined, or wrapped) are skipped, while any tree containing a real error —
// including mixed joins of sentinel and real, which setup.go's concurrent shutdown
// naturally produces when one provider stalls on a canceled ctx while another fails with a
// real export error — is still captured by Sentry.
func TestShutdownSkipsSentryForJoinedCancellation(t *testing.T) {
	var captured []error
	prev := sentrygo.CurrentHub().Client()
	require.NoError(t, sentrygo.Init(sentrygo.ClientOptions{
		DisableTelemetryBuffer: true,
		BeforeSend: func(_ *sentrygo.Event, hint *sentrygo.EventHint) *sentrygo.Event {
			captured = append(captured, hint.OriginalException)
			return nil // Drop the event.
		},
	}))
	t.Cleanup(func() { sentrygo.CurrentHub().BindClient(prev) })

	ph, err := posthog.New(posthog.Options{})
	require.NoError(t, err)

	for _, tc := range shutdownErrorCases {
		want := 0
		if !tc.sentinel {
			want = 1
		}
		t.Run(tc.name, func(t *testing.T) {
			captured = nil
			tel := Telemetry{posthog: ph, shutdown: func(context.Context) error { return tc.err }}
			require.ErrorIs(t, tel.Shutdown(t.Context()), tc.err)
			require.Len(t, captured, want, tc.err.Error())
		})
	}
}

// TestShutdownSkipsSentryForStalledMetricsTimeout reproduces the pure-timeout shutdown a
// stalled metrics collector produces: the OTLP gRPC exporter returns a
// [codes.DeadlineExceeded] status joined with [context.DeadlineExceeded]. Both branches
// are cancellation sentinels, so the guard must keep it out of Sentry — the regression the
// gRPC status normalization protects against.
func TestShutdownSkipsSentryForStalledMetricsTimeout(t *testing.T) {
	var captured []error
	prev := sentrygo.CurrentHub().Client()
	require.NoError(t, sentrygo.Init(sentrygo.ClientOptions{
		DisableTelemetryBuffer: true,
		BeforeSend: func(_ *sentrygo.Event, hint *sentrygo.EventHint) *sentrygo.Event {
			captured = append(captured, hint.OriginalException)
			return nil
		},
	}))
	t.Cleanup(func() { sentrygo.CurrentHub().BindClient(prev) })

	addr := startStalledMetricsServer(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", addr)
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", addr)
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("LOG_FORMAT", "json")
	restoreGlobalProviders(t)

	tel, err := New(Options{ServiceName: "test"})
	require.NoError(t, err)
	counter, err := otel.Meter("test").Int64Counter("test.requests")
	require.NoError(t, err)
	counter.Add(context.Background(), 1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tel.Shutdown(ctx)
	require.Empty(t, captured, "pure-timeout shutdown must not reach Sentry")
}

// TestOnlyContextSentinels pins the behavior of the shutdown-capture guard's purity check
// directly, independent of the Sentry integration. It verifies that the helper returns
// true only when every branch of the error tree is a context sentinel, and false as soon
// as any branch is a real error — including mixed joins and wrapped variants that
// setup.go's concurrent shutdown produces.
func TestOnlyContextSentinels(t *testing.T) {
	cases := make([]struct {
		name     string
		err      error
		sentinel bool
	}, 0, len(shutdownErrorCases)+3)
	cases = append(cases, shutdownErrorCases...)
	cases = append(cases,
		struct {
			name     string
			err      error
			sentinel bool
		}{name: "nil", err: nil, sentinel: true},
		struct {
			name     string
			err      error
			sentinel bool
		}{name: "join with nil and real", err: errors.Join(nil, errShutdownReal), sentinel: false},
		struct {
			name     string
			err      error
			sentinel bool
		}{name: "join with nil and Canceled", err: errors.Join(nil, context.Canceled), sentinel: true},
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.sentinel, onlyContextSentinels(tc.err))
		})
	}
}
