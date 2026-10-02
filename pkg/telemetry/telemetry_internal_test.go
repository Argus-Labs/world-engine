package telemetry

import (
	"testing"

	"github.com/stretchr/testify/require"
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
