package telemetry

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

// tracesExporterTarget follows the OTel env spec for precedence and URL schemes, except that a bare
// host:port defaults to plaintext so the stock jaeger:4317 collector keeps working.
func TestTracesExporterTarget(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want exporterTarget
	}{
		{
			name: "default bare endpoint is plaintext",
			cfg:  Config{Endpoint: "jaeger:4317"},
			want: exporterTarget{Endpoint: "jaeger:4317", Insecure: true},
		},
		{
			name: "traces endpoint wins over generic endpoint",
			cfg:  Config{Endpoint: "generic:4317", TracesEndpoint: "traces:4317"},
			want: exporterTarget{Endpoint: "traces:4317", Insecure: true},
		},
		{
			name: "https URL is secure",
			cfg:  Config{Endpoint: "https://otel.example.com:4317"},
			want: exporterTarget{Endpoint: "https://otel.example.com:4317", Insecure: false},
		},
		{
			name: "http URL is plaintext",
			cfg:  Config{Endpoint: "http://otel.example.com:4317"},
			want: exporterTarget{Endpoint: "http://otel.example.com:4317", Insecure: true},
		},
		{
			name: "https traces URL wins over generic bare endpoint",
			cfg:  Config{Endpoint: "jaeger:4317", TracesEndpoint: "https://otel.example.com/v1/traces"},
			want: exporterTarget{Endpoint: "https://otel.example.com/v1/traces", Insecure: false},
		},
		{
			name: "URL scheme ignores insecure flags",
			cfg:  Config{Endpoint: "https://otel.example.com:4317", Insecure: boolPtr(true), TracesInsecure: boolPtr(true)},
			want: exporterTarget{Endpoint: "https://otel.example.com:4317", Insecure: false},
		},
		{
			name: "insecure=false on bare endpoint selects TLS",
			cfg:  Config{Endpoint: "otel.example.com:4317", Insecure: boolPtr(false)},
			want: exporterTarget{Endpoint: "otel.example.com:4317", Insecure: false},
		},
		{
			name: "insecure=true on bare endpoint selects plaintext",
			cfg:  Config{Endpoint: "otel.example.com:4317", Insecure: boolPtr(true)},
			want: exporterTarget{Endpoint: "otel.example.com:4317", Insecure: true},
		},
		{
			name: "traces insecure=false wins over insecure=true",
			cfg:  Config{Endpoint: "otel.example.com:4317", Insecure: boolPtr(true), TracesInsecure: boolPtr(false)},
			want: exporterTarget{Endpoint: "otel.example.com:4317", Insecure: false},
		},
		{
			name: "traces insecure=true wins over insecure=false",
			cfg:  Config{Endpoint: "otel.example.com:4317", Insecure: boolPtr(false), TracesInsecure: boolPtr(true)},
			want: exporterTarget{Endpoint: "otel.example.com:4317", Insecure: true},
		},
		{
			name: "empty endpoint stays empty",
			cfg:  Config{Endpoint: ""},
			want: exporterTarget{Endpoint: "", Insecure: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.cfg.tracesExporterTarget())
		})
	}
}

// The same rules hold end to end through the environment variables and into Options.
func TestLoadConfig_ResolvesExporterTargetFromEnv(t *testing.T) {
	tests := []struct {
		name         string
		env          map[string]string
		wantEndpoint string
		wantInsecure bool
	}{
		{
			name:         "no variables use the plaintext default",
			env:          map[string]string{},
			wantEndpoint: "jaeger:4317",
			wantInsecure: true,
		},
		{
			name: "traces endpoint wins over generic endpoint",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT":        "generic:4317",
				"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "traces:4317",
			},
			wantEndpoint: "traces:4317",
			wantInsecure: true,
		},
		{
			name:         "https URL is secure",
			env:          map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "https://otel.example.com:4317"},
			wantEndpoint: "https://otel.example.com:4317",
			wantInsecure: false,
		},
		{
			name: "insecure=false on bare endpoint selects TLS",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "otel.example.com:4317",
				"OTEL_EXPORTER_OTLP_INSECURE": "false",
			},
			wantEndpoint: "otel.example.com:4317",
			wantInsecure: false,
		},
		{
			name: "traces insecure wins over generic insecure",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT":        "otel.example.com:4317",
				"OTEL_EXPORTER_OTLP_INSECURE":        "false",
				"OTEL_EXPORTER_OTLP_TRACES_INSECURE": "true",
			},
			wantEndpoint: "otel.example.com:4317",
			wantInsecure: true,
		},
		{
			name: "empty insecure value keeps the plaintext default",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "otel.example.com:4317",
				"OTEL_EXPORTER_OTLP_INSECURE": "",
			},
			wantEndpoint: "otel.example.com:4317",
			wantInsecure: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{
				"OTEL_EXPORTER_OTLP_ENDPOINT",
				"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
				"OTEL_EXPORTER_OTLP_INSECURE",
				"OTEL_EXPORTER_OTLP_TRACES_INSECURE",
			} {
				t.Setenv(key, "")
				if _, ok := tt.env[key]; !ok {
					// t.Setenv registered the restore; clear the variable so the default applies.
					require.NoError(t, os.Unsetenv(key))
				}
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			cfg, err := loadConfig()
			require.NoError(t, err)
			opts := newDefaultOptions()
			cfg.applyToOptions(&opts)

			require.Equal(t, tt.wantEndpoint, opts.Endpoint)
			require.Equal(t, tt.wantInsecure, opts.Insecure)
		})
	}
}

// A URL endpoint that the exporter would silently replace with its built-in default is rejected up front.
func TestLoadConfig_RejectsURLEndpointWithoutHost(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://")

	_, err := loadConfig()
	require.ErrorContains(t, err, `invalid OTLP endpoint URL "https://": missing host`)
}
