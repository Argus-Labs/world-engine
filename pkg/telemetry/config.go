package telemetry

import (
	"strings"

	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	"github.com/argus-labs/world-engine/pkg/telemetry/sentry"
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

	// Log format configuration ("json", "pretty").
	LogFormat string `env:"OTEL_LOG_FORMAT" envDefault:"json"`

	// SentryDsn is the Sentry DSN.
	SentryDsn string `env:"OTEL_SENTRY_DSN"`

	// SentryEnvironment is to determine if shard is running in development or production (DEV/PROD).
	SentryENV string `env:"OTEL_SENTRY_ENV"`

	// PosthogAPIKey is the PostHog API key.
	PosthogAPIKey string `env:"OTEL_POSTHOG_API_KEY"`
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

	// Validate log format.
	if ParseLogFormat(cfg.LogFormat) == LogFormatUndefined {
		return eris.Errorf("invalid log format: %s (must be 'json' or 'pretty')", cfg.LogFormat)
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
	opt.LogFormat = ParseLogFormat(cfg.LogFormat)
	opt.TraceSampleRate = cfg.TraceSampleRate
	opt.SentryOptions = sentry.Options{
		Dsn:         cfg.SentryDsn,
		Environment: cfg.SentryENV,
	}
	opt.PosthogOptions = posthog.Options{
		APIKey: cfg.PosthogAPIKey,
	}
}

type Options struct {
	ServiceName     string // Name of the service for telemetry
	Endpoint        string
	LogLevel        string
	LogFormat       LogFormat // Log output format
	TraceSampleRate float64

	SentryOptions  sentry.Options
	PosthogOptions posthog.Options
}

func newDefaultOptions() Options {
	// Set these to invalid values to force users to pass in the correct options.
	return Options{
		Endpoint:        "",
		ServiceName:     "",
		LogLevel:        "",
		LogFormat:       LogFormatUndefined,
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
	if newOpt.LogFormat != LogFormatUndefined {
		opt.LogFormat = newOpt.LogFormat
	}
	if newOpt.TraceSampleRate != 0.0 {
		opt.TraceSampleRate = newOpt.TraceSampleRate
	}
	if newOpt.SentryOptions.Tags != nil {
		opt.SentryOptions.Tags = newOpt.SentryOptions.Tags
	}
	if newOpt.PosthogOptions.DistinctID != "" {
		opt.PosthogOptions.DistinctID = newOpt.PosthogOptions.DistinctID
	}
	if newOpt.PosthogOptions.BaseProperties != nil {
		opt.PosthogOptions.BaseProperties = newOpt.PosthogOptions.BaseProperties
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
	if opt.LogFormat == LogFormatUndefined {
		return eris.New("log format must be specified")
	}
	if opt.TraceSampleRate < 0.0 || opt.TraceSampleRate > 1.0 {
		return eris.New("trace sample rate must be between 0.0 and 1.0")
	}
	return nil
}

// LogFormat represents the log output format.
type LogFormat uint8

const (
	LogFormatUndefined LogFormat = iota // Used as the zero value
	LogFormatJSON                       // Outputs structured JSON logs
	LogFormatPretty                     // Outputs human-readable console logs
)

const (
	jsonFormatString      = "json"
	prettyFormatString    = "pretty"
	undefinedFormatString = "undefined"
)

func (f LogFormat) String() string {
	switch f {
	case LogFormatUndefined:
		return undefinedFormatString
	case LogFormatJSON:
		return jsonFormatString
	case LogFormatPretty:
		return prettyFormatString
	default:
		return undefinedFormatString
	}
}

// ParseLogFormat converts a string to LogFormat enum.
func ParseLogFormat(s string) LogFormat {
	switch strings.ToLower(s) {
	case jsonFormatString:
		return LogFormatJSON
	case prettyFormatString:
		return LogFormatPretty
	default:
		return LogFormatUndefined
	}
}
