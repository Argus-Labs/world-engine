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
)

const (
	RunModeProd RunMode = "production"
	RunModeDev  RunMode = "development"

	// Default configuration values.

	DefaultRunMode       = RunModeDev
	DefaultNamespace     = "world-1"
	DefaultRedisAddress  = "localhost:6379"
	DefaultLogLevel      = "info"
	DefaultStatsdAddress = "localhost:8125"
)

var validLogLevels = []string{
	zerolog.DebugLevel.String(),
	zerolog.InfoLevel.String(),
	zerolog.WarnLevel.String(),
	zerolog.ErrorLevel.String(),
	zerolog.Disabled.String(),
}

var defaultConfig = WorldConfig{
	RedisAddress:              DefaultRedisAddress,
	RedisPassword:             "",
	CardinalNamespace:         DefaultNamespace,
	CardinalMode:              DefaultRunMode,
	BaseShardSequencerAddress: "",
	BaseShardQueryAddress:     "",
	CardinalLogLevel:          DefaultLogLevel,
	StatsdAddress:             DefaultStatsdAddress,
	TraceAddress:              "",
}

type RunMode string

type WorldConfig struct {
	// Cardinal
	CardinalMode      RunMode `config:"CARDINAL_MODE"`
	CardinalNamespace string  `config:"CARDINAL_NAMESPACE"`
	CardinalLogLevel  string  `config:"CARDINAL_LOG_LEVEL"`

	// Redis
	RedisAddress  string `config:"REDIS_ADDRESS"`
	RedisPassword string `config:"REDIS_PASSWORD"`

	// Shard networking
	BaseShardSequencerAddress string `config:"BASE_SHARD_SEQUENCER_ADDRESS"`
	BaseShardQueryAddress     string `config:"BASE_SHARD_QUERY_ADDRESS"`

	// Telemetry
	StatsdAddress string `config:"STATSD_ADDRESS"`
	TraceAddress  string `config:"TRACE_ADDRESS"`
	// RouterKey is a token used to secure communications between the game shard and the base shard.
	RouterKey string `config:"ROUTER_KEY"`
}

func loadWorldConfig() (*WorldConfig, error) {
	cfg := defaultConfig

	// Load config from environment variables
	if err := config.FromEnv().To(&cfg); err != nil {
		return nil, eris.Wrap(err, "Failed to load config")
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, eris.Wrap(err, "Invalid config")
	}

	// Set logger config
	if err := cfg.setLogger(); err != nil {
		return nil, eris.Wrap(err, "Failed to set log level")
	}

	return &cfg, nil
}

// Validate validates the config values and ensures that when the RunMode is production, the required values are set.
//
//nolint:gocognit // its fine.
func (w *WorldConfig) Validate() error {
	// Validate run mode
	if w.CardinalMode != RunModeProd && w.CardinalMode != RunModeDev {
		return eris.Errorf("CARDINAL_MODE must be either %q or %q", RunModeProd, RunModeDev)
	}

	// Validate production mode configs
	if w.CardinalMode == RunModeProd {
		// Validate that Redis password is set
		if w.RedisPassword == "" {
			return eris.New("REDIS_PASSWORD must be set in production mode")
		}
		// Validate shard networking config
		if _, _, err := net.SplitHostPort(w.BaseShardSequencerAddress); err != nil {
			return eris.Wrap(err, "BASE_SHARD_SEQUENCER_ADDRESS must follow the format <host>:<port>")
		}
		if _, _, err := net.SplitHostPort(w.BaseShardQueryAddress); err != nil {
			return eris.Wrap(err, "BASE_SHARD_QUERY_ADDRESS must follow the format <host>:<port>")
		}
		if w.RouterKey == "" {
			return eris.New("ROUTER_KEY must be set in production mode")
		}
	}

	// Validate Cardinal configs
	if err := Namespace(w.CardinalNamespace).Validate(w.CardinalMode); err != nil {
		return eris.Wrap(err, "CARDINAL_NAMESPACE is not a valid namespace")
	}
	if w.CardinalLogLevel == "" || !slices.Contains(validLogLevels, w.CardinalLogLevel) {
		return eris.New("CARDINAL_LOG_LEVEL must be one of the following: " + strings.Join(validLogLevels, ", "))
	}

	// Validate Redis address
	if _, _, err := net.SplitHostPort(w.RedisAddress); err != nil {
		return eris.New("REDIS_ADDRESS must follow the format <host>:<port>")
	}

	// Validate telemetry config
	if w.StatsdAddress != "" {
		if _, _, err := net.SplitHostPort(w.StatsdAddress); err != nil {
			return eris.New("STATSD_ADDRESS must follow the format <host>:<port>")
		}
	}
	if w.TraceAddress != "" {
		if _, _, err := net.SplitHostPort(w.TraceAddress); err != nil {
			return eris.New("TRACE_ADDRESS must follow the format <host>:<port>")
		}
	}

	return nil
}

func (w *WorldConfig) setLogger() error {
	// Parse the log level
	level, err := zerolog.ParseLevel(w.CardinalLogLevel)
	if err != nil {
		return eris.Wrap(err, "CARDINAL_LOG_LEVEL is not a valid log level")
	}

	// Set global logger level
	zerolog.SetGlobalLevel(level)

	// Enable pretty logging in development mode
	if w.CardinalMode == RunModeDev {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	return nil
}
