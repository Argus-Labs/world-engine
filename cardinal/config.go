package cardinal

import (
	"net"
	"os"
	"slices"
	"strings"

	"github.com/JeremyLoy/config"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/rift/credentials"
)

const (
	DefaultCardinalNamespace         = "world-1"
	DefaultCardinalLogLevel          = "info"
	DefaultRedisAddress              = "localhost:6379"
	DefaultBaseShardSequencerAddress = "localhost:9601"
)

var (
	validLogLevels = []string{
		zerolog.DebugLevel.String(),
		zerolog.InfoLevel.String(),
		zerolog.WarnLevel.String(),
		zerolog.ErrorLevel.String(),
		zerolog.Disabled.String(),
	}

	defaultConfig = WorldConfig{
		CardinalNamespace:         DefaultCardinalNamespace,
		CardinalRollupEnabled:     false,
		CardinalLogPretty:         false,
		CardinalLogLevel:          DefaultCardinalLogLevel,
		RedisAddress:              DefaultRedisAddress,
		RedisPassword:             "",
		BaseShardSequencerAddress: DefaultBaseShardSequencerAddress,
		BaseShardRouterKey:        "",
		TelemetryTraceEnabled:     false,
		TelemetryProfilerEnabled:  false,
	}
)

type WorldConfig struct {
	// CardinalNamespace The shard namespace for Cardinal. This needs to be unique to prevent signature replay attacks.
	CardinalNamespace string `config:"CARDINAL_NAMESPACE"`

	// CardinalRollupEnabled When true, Cardinal will sequence and recover to/from base shard.
	CardinalRollupEnabled bool `config:"CARDINAL_ROLLUP_ENABLED"`

	// CardinalLogLevel Determines the log level for Cardinal.
	CardinalLogLevel string `config:"CARDINAL_LOG_LEVEL"`

	// CardinalLogPretty Pretty logging, disable by default due to performance impact.
	CardinalLogPretty bool `config:"CARDINAL_LOG_PRETTY"`

	// RedisAddress The address of the redis server, supports unix sockets.
	RedisAddress string `config:"REDIS_ADDRESS"`

	// RedisPassword The password for the redis server. Make sure to use a password in production.
	RedisPassword string `config:"REDIS_PASSWORD"`

	// BaseShardSequencerAddress This is the address that Cardinal will use to sequence and recover to/from base shard.
	BaseShardSequencerAddress string `config:"BASE_SHARD_SEQUENCER_ADDRESS"`

	// BaseShardRouterKey is a token used to secure communications between the game shard and the base shard.
	BaseShardRouterKey string `config:"BASE_SHARD_ROUTER_KEY"`

	// TelemetryTraceEnabled When true, Cardinal will collect OpenTelemetry traces
	TelemetryTraceEnabled bool `config:"TELEMETRY_TRACE_ENABLED"`

	// TelemetryProfilerEnabled When true, Cardinal will run Datadog continuous profiling
	TelemetryProfilerEnabled bool `config:"TELEMETRY_PROFILER_ENABLED"`
}

func loadWorldConfig() (*WorldConfig, error) {
	cfg := defaultConfig

	if err := config.FromEnv().To(&cfg); err != nil {
		return nil, eris.Wrap(err, "Failed to load config")
	}

	if err := cfg.Validate(); err != nil {
		return nil, eris.Wrap(err, "Invalid config")
	}

	if err := cfg.setLogger(); err != nil {
		return nil, eris.Wrap(err, "Failed to set log level")
	}

	return &cfg, nil
}

// Validate validates the config values.
// If CARDINAL_ROLLUP=true, the BASE_SHARD_SEQUENCER_ADDRESS and BASE_SHARD_ROUTER_KEY are required.
func (w *WorldConfig) Validate() error {
	// Validate Cardinal configs
	if err := Namespace(w.CardinalNamespace).Validate(); err != nil {
		return eris.Wrap(err, "CARDINAL_NAMESPACE is not a valid namespace")
	}
	if w.CardinalLogLevel == "" || !slices.Contains(validLogLevels, w.CardinalLogLevel) {
		return eris.New("CARDINAL_LOG_LEVEL must be one of the following: " + strings.Join(validLogLevels, ", "))
	}

	// Validate Redis address
	if _, _, err := net.SplitHostPort(w.RedisAddress); err != nil {
		return eris.New("REDIS_ADDRESS must follow the format <host>:<port>")
	}

	// Validate base shard configs (only required when rollup mode is enabled)
	if w.CardinalRollupEnabled {
		if _, _, err := net.SplitHostPort(w.BaseShardSequencerAddress); err != nil {
			return eris.Wrap(err, "BASE_SHARD_SEQUENCER_ADDRESS must follow the format <host>:<port>")
		}
		if w.BaseShardRouterKey == "" {
			return eris.New("BASE_SHARD_ROUTER_KEY must be when rollup mode is enabled")
		}
		if err := credentials.ValidateKey(w.BaseShardRouterKey); err != nil {
			return err
		}
	}

	return nil
}

func (w *WorldConfig) setLogger() error {
	// Set global logger level
	level, err := zerolog.ParseLevel(w.CardinalLogLevel)
	if err != nil {
		return eris.Wrap(err, "CARDINAL_LOG_LEVEL is not a valid log level")
	}
	zerolog.SetGlobalLevel(level)

	// Override global logger to console writer if pretty logging is enabled
	if w.CardinalLogPretty {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	return nil
}
