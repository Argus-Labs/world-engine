package telemetry

import (
	"net/url"
	"strings"

	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	"github.com/argus-labs/world-engine/pkg/telemetry/sentry"
	"github.com/caarlos0/env/v11"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

type Config struct {
	// Endpoint is the OTLP collector endpoint, either a bare host:port or a URL with a scheme.
	Endpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:"jaeger:4317"`

	// TracesEndpoint is the signal-specific OTLP endpoint; when set it takes precedence over Endpoint.
	TracesEndpoint string `env:"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"`

	// Insecure selects plaintext gRPC for a bare host:port endpoint. Ignored for URL endpoints,
	// where the scheme decides. Unset keeps the plaintext default.
	Insecure *bool `env:"OTEL_EXPORTER_OTLP_INSECURE"`

	// TracesInsecure is the signal-specific form of Insecure; when set it takes precedence.
	TracesInsecure *bool `env:"OTEL_EXPORTER_OTLP_TRACES_INSECURE"`

	// TraceSampleRate is the sampling rate for traces (0.0 to 1.0).
	TraceSampleRate float64 `env:"OTEL_TRACE_SAMPLE_RATE" envDefault:"1.0"`

	// Log level configuration ("debug", "info", "warn", "error").
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	// Log format configuration ("json", "pretty").
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"`

	// SentryDsn is the Sentry DSN.
	SentryDsn string `env:"SENTRY_DSN"`

	// SentryEnvironment is to determine if shard is running in development or production (DEV/PROD).
	SentryEnvironment string `env:"SENTRY_ENVIRONMENT"`

	// PosthogAPIKey is the PostHog API key.
	PosthogAPIKey string `env:"POSTHOG_API_KEY"`
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

	// Validate OTLP configuration if endpoint is set
	target := cfg.tracesExporterTarget()
	if target.Endpoint != "" {
		if cfg.TraceSampleRate < 0.0 || cfg.TraceSampleRate > 1.0 {
			return eris.New("trace sample rate must be between 0.0 and 1.0")
		}
		if err := target.validate(); err != nil {
			return err
		}
	}

	return nil
}

// exporterTarget is the resolved OTLP trace exporter destination.
type exporterTarget struct {
	// Endpoint is either a bare host:port or a URL with a scheme.
	Endpoint string
	// Insecure reports whether the exporter dials plaintext gRPC.
	Insecure bool
}

// isURL reports whether the endpoint carries a scheme, in which case the scheme decides transport security.
func (t exporterTarget) isURL() bool {
	return strings.Contains(t.Endpoint, "://")
}

// validate rejects URL endpoints that the exporter would silently replace with its built-in default.
func (t exporterTarget) validate() error {
	if !t.isURL() {
		return nil
	}
	u, err := url.Parse(t.Endpoint)
	if err != nil {
		return eris.Wrapf(err, "invalid OTLP endpoint URL %q", t.Endpoint)
	}
	if u.Host == "" {
		return eris.Errorf("invalid OTLP endpoint URL %q: missing host", t.Endpoint)
	}
	return nil
}

// tracesExporterTarget resolves the endpoint and transport security for the trace exporter following the
// OTel SDK environment variable spec, with one deliberate deviation: a bare host:port defaults to plaintext
// (the spec defaults to TLS) because the default endpoint and in-cluster collectors speak plaintext gRPC.
//
//   - OTEL_EXPORTER_OTLP_TRACES_ENDPOINT wins over OTEL_EXPORTER_OTLP_ENDPOINT.
//   - A URL endpoint is secure iff its scheme is https; the insecure flags are ignored.
//   - A bare host:port honors OTEL_EXPORTER_OTLP_TRACES_INSECURE, then OTEL_EXPORTER_OTLP_INSECURE.
func (cfg *Config) tracesExporterTarget() exporterTarget {
	target := exporterTarget{Endpoint: cfg.Endpoint, Insecure: true}
	if cfg.TracesEndpoint != "" {
		target.Endpoint = cfg.TracesEndpoint
	}
	if target.isURL() {
		target.Insecure = !strings.HasPrefix(strings.ToLower(target.Endpoint), "https://")
		return target
	}
	if cfg.Insecure != nil {
		target.Insecure = *cfg.Insecure
	}
	if cfg.TracesInsecure != nil {
		target.Insecure = *cfg.TracesInsecure
	}
	return target
}

func (cfg *Config) applyToOptions(opt *Options) {
	target := cfg.tracesExporterTarget()
	opt.Endpoint = target.Endpoint
	opt.Insecure = target.Insecure
	opt.LogLevel = cfg.LogLevel
	opt.LogFormat = ParseLogFormat(cfg.LogFormat)
	opt.TraceSampleRate = cfg.TraceSampleRate
	opt.SentryOptions = sentry.Options{
		Dsn:         cfg.SentryDsn,
		Environment: cfg.SentryEnvironment,
	}
	opt.PosthogOptions = posthog.Options{
		APIKey: cfg.PosthogAPIKey,
	}
}

type Options struct {
	ServiceName     string // Name of the service for telemetry
	Endpoint        string // OTLP endpoint, a bare host:port or a URL with a scheme
	Insecure        bool   // Plaintext gRPC for a bare host:port endpoint; ignored for URLs
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
	_, err := zerolog.ParseLevel(strings.ToLower(opt.LogLevel))
	if err != nil {
		return eris.Errorf("invalid log level: %s (must be 'debug', 'info', 'warn', or 'error')", opt.LogLevel)
	}
	if opt.LogFormat == LogFormatUndefined {
		return eris.New("log format must be specified")
	}
	if opt.Endpoint != "" {
		if opt.TraceSampleRate < 0.0 || opt.TraceSampleRate > 1.0 {
			return eris.New("trace sample rate must be between 0.0 and 1.0")
		}
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
