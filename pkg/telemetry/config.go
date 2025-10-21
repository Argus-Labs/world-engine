package telemetry

import (
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

type Config struct {
	// Enabled when false disables OpenTelemetry entirely.
	Enabled bool `env:"OTEL_ENABLED" envDefault:"true"`

	// Endpoint is the OTLP collector endpoint.
	Endpoint string `env:"OTEL_ENDPOINT" envDefault:"jaeger:4317"`

	// TraceSampleRate is the sampling rate for traces (0.0 to 1.0).
	TraceSampleRate float64 `env:"OTEL_TRACE_SAMPLE_RATE" envDefault:"1.0"`

	// Log level configuration ("debug", "info", "warn", "error").
	LogLevel string `env:"OTEL_LOG_LEVEL" envDefault:"info"`
}

// LoadConfig loads the configuration from environment variables.
func loadConfig() (Config, error) {
	cfg := Config{}

	if err := env.Parse(&cfg); err != nil {
		return cfg, eris.Wrap(err, "failed to parse telemetry config")
	}

	if err := cfg.validate(); err != nil {
		return cfg, eris.Wrap(err, "failed to validate telemetry config")
	}

	return cfg, nil
}

// validate performs validation on the loaded configuration.
func (cfg *Config) validate() error {
	// Validate log level.
	_, err := zerolog.ParseLevel(strings.ToLower(cfg.LogLevel))
	if err != nil {
		return eris.Errorf("invalid log level: %s (must be 'debug', 'info', 'warn', or 'error')", cfg.LogLevel)
	}

	// Validate OTLP configuration
	if cfg.Enabled {
		if cfg.Endpoint == "" {
			return eris.New("OTLP endpoint cannot be empty when OTLP is enabled")
		}

		if cfg.TraceSampleRate < 0.0 || cfg.TraceSampleRate > 1.0 {
			return eris.New("trace sample rate must be between 0.0 and 1.0")
		}
	}

	return nil
}

func (cfg *Config) applyToOptions(opt *Options) {
	opt.Endpoint = cfg.Endpoint
	opt.LogLevel = cfg.LogLevel
	opt.TraceSampleRate = cfg.TraceSampleRate
}

type Options struct {
	ServiceName     string // Name of the service for telemetry
	Endpoint        string
	LogLevel        string
	TraceSampleRate float64
}

func newDefaultOptions() Options {
	// Set these to invalid values to force users to pass in the correct options.
	return Options{
		Endpoint:        "",
		ServiceName:     "",
		LogLevel:        "",
		TraceSampleRate: -1.0,
	}
}

// apply merges the given options into the current options, overriding non-zero values.
func (opt *Options) apply(newOpt Options) {
	if newOpt.ServiceName != "" {
		opt.ServiceName = newOpt.ServiceName
	}
	if newOpt.LogLevel != "" {
		opt.LogLevel = newOpt.LogLevel
	}
	if newOpt.TraceSampleRate != 0.0 {
		opt.TraceSampleRate = newOpt.TraceSampleRate
	}
}

// validate checks that all required options are set and valid.
func (opt *Options) validate() error {
	if opt.ServiceName == "" {
		return eris.New("service name cannot be empty")
	}
	if opt.Endpoint == "" {
		return eris.New("endpoint cannot be empty")
	}
	_, err := zerolog.ParseLevel(strings.ToLower(opt.LogLevel))
	if err != nil {
		return eris.Errorf("invalid log level: %s (must be 'debug', 'info', 'warn', or 'error')", opt.LogLevel)
	}
	if opt.TraceSampleRate < 0.0 || opt.TraceSampleRate > 1.0 {
		return eris.New("trace sample rate must be between 0.0 and 1.0")
	}
	return nil
}
