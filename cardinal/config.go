package cardinal

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"pkg.world.dev/world-engine/rift/credentials"
)

const (
	DefaultCardinalNamespace         = "world-1"
	DefaultCardinalLogLevel          = "info"
	DefaultRedisAddress              = "localhost:6379"
	DefaultBaseShardSequencerAddress = "localhost:9601"

	// Toml config file related
	configFilePathEnvVariable = "CARDINAL_CONFIG"
	defaultConfigFileName     = "world.toml"
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
		CardinalTickRate:          0,
	}
)

type WorldConfig struct {
	// CardinalNamespace The shard namespace for Cardinal. This needs to be unique to prevent signature replay attacks.
	CardinalNamespace string `mapstructure:"CARDINAL_NAMESPACE"`

	// CardinalRollupEnabled When true, Cardinal will sequence and recover to/from base shard.
	CardinalRollupEnabled bool `mapstructure:"CARDINAL_ROLLUP_ENABLED"`

	// CardinalLogLevel Determines the log level for Cardinal.
	CardinalLogLevel string `mapstructure:"CARDINAL_LOG_LEVEL"`

	// CardinalLogPretty Pretty logging, disable by default due to performance impact.
	CardinalLogPretty bool `mapstructure:"CARDINAL_LOG_PRETTY"`

	// RedisAddress The address of the redis server, supports unix sockets.
	RedisAddress string `mapstructure:"REDIS_ADDRESS"`

	// RedisPassword The password for the redis server. Make sure to use a password in production.
	RedisPassword string `mapstructure:"REDIS_PASSWORD"`

	// BaseShardSequencerAddress This is the address that Cardinal will use to sequence and recover to/from base shard.
	BaseShardSequencerAddress string `mapstructure:"BASE_SHARD_SEQUENCER_ADDRESS"`

	// BaseShardRouterKey is a token used to secure communications between the game shard and the base shard.
	BaseShardRouterKey string `mapstructure:"BASE_SHARD_ROUTER_KEY"`

	// TelemetryTraceEnabled When true, Cardinal will collect OpenTelemetry traces
	TelemetryTraceEnabled bool `mapstructure:"TELEMETRY_TRACE_ENABLED"`

	// CardinalTickRate The number of ticks per second
	CardinalTickRate uint64 `mapstructure:"CARDINAL_TICK_RATE"`
}

func loadWorldConfig() (*WorldConfig, error) {
	// Set default config
	cfg := defaultConfig

	// Setup Viper for world toml config file
	setupViper()

	// Read the config file
	// Unmarshal the [cardinal] section from config file into the WorldConfig struct
	if err := viper.ReadInConfig(); err != nil {
		log.Warn().Err(err).Msg("No config file found")
	} else {
		if err := viper.Sub("cardinal").Unmarshal(&cfg); err != nil {
			log.Warn().Err(err).Msg("Failed to unmarshal config file")
		}
	}

	// Override config values with environment variables
	// This is done after reading the config file to allow for environment variable overrides
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Warn().Err(err).Msg("Failed to load config from environment variables")
	} else {
		log.Debug().Msg("Loaded config from environment variables")
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

func setupViper() {
	if pflag.Lookup(configFilePathEnvVariable) == nil {
		pflag.String(configFilePathEnvVariable, "", "Path to the TOML config file")
	}

	pflag.Parse()

	// Bind the command-line flags to Viper
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Debug().Err(err).Msg("Failed to bind command-line flags to Viper")
		// Continue even if the binding fails
	}

	// Bind env for CARDINAL_CONFIG
	if err := viper.BindEnv(configFilePathEnvVariable); err != nil {
		log.Warn().Err(err).Str("env", configFilePathEnvVariable).Msg("Failed to bind env variable")
	}

	// Set default toml config file name and type
	viper.SetConfigName("world") // name of config file (without extension)
	viper.SetConfigType("toml")  // REQUIRED if the config file does not have the extension in the name

	// Find the toml config file from the flag and env variable
	// viper precedence: flag > env > default
	configFilePath := viper.GetString(configFilePathEnvVariable)
	if configFilePath != "" { //nolint:nestif // better consistency and readability
		// Use Specified config file
		fileName := filepath.Base(configFilePath)

		viper.SetConfigName(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
		viper.SetConfigType(strings.TrimPrefix(filepath.Ext(fileName), "."))

		viper.AddConfigPath(filepath.Dir(configFilePath))
	} else {
		// Search for toml file in the current directory and parent directory
		viper.AddConfigPath(".") // look for config in the working directory

		// If the config file is not found in the current directory, search in the parent directory
		if _, err := os.Stat(defaultConfigFileName); err != nil {
			parentDir, err := os.Getwd()
			if err != nil {
				log.Warn().Err(err).Msg("Failed to get current directory for TOML file search")
			} else {
				parentDir = filepath.Dir(parentDir) // get parent directory
				viper.AddConfigPath(parentDir)
			}
		}
	}

	// Bind env from struct tags
	val := reflect.ValueOf(&defaultConfig).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("mapstructure")
		if tag != "" {
			if err := viper.BindEnv(tag); err != nil {
				log.Warn().Err(err).Str("field", field.Name).Msg("Failed to bind env variable")
			}
		}
	}
}
