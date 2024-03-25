package cardinal

import (
	"github.com/JeremyLoy/config"
	"github.com/rotisserie/eris"
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
	RedisAddress              string  `config:"REDIS_ADDRESS"`
	RedisPassword             string  `config:"REDIS_PASSWORD"`
	CardinalNamespace         string  `config:"CARDINAL_NAMESPACE"`
	CardinalMode              RunMode `config:"CARDINAL_MODE"`
	BaseShardSequencerAddress string  `config:"BASE_SHARD_SEQUENCER_ADDRESS"`
	BaseShardQueryAddress     string  `config:"BASE_SHARD_QUERY_ADDRESS"`
	CardinalLogLevel          string  `config:"CARDINAL_LOG_LEVEL"`
	StatsdAddress             string  `config:"STATSD_ADDRESS"`
	TraceAddress              string  `config:"TRACE_ADDRESS"`
	// RouterKey is a JWT token used to secure communications between the game shard and the base shard.
	RouterKey string `config:"ROUTER_KEY"`
}

func (w WorldConfig) Validate() error {
	if w.CardinalMode != RunModeProd {
		return nil
	}
	if w.RedisPassword == "" {
		return eris.New("REDIS_PASSWORD is required in production")
	}
	if w.CardinalNamespace == DefaultNamespace {
		return eris.New("CARDINAL_NAMESPACE cannot be the default value in production to avoid replay attack")
	}
	if w.BaseShardSequencerAddress == "" || w.BaseShardQueryAddress == "" {
		return eris.New("must supply BASE_SHARD_SEQUENCER_ADDRESS and BASE_SHARD_QUERY_ADDRESS for production " +
			"mode Cardinal worlds")
	}
	if w.RouterKey == "" {
		return eris.New("must supply ROUTER_KEY for production mode Cardinal worlds")
	}
	return nil
}

func getWorldConfig() WorldConfig {
	cfg := defaultConfig
	err := config.FromEnv().To(&cfg)
	if err != nil {
		panic(err)
	}
	return cfg
}
