package cardinal

import (
	"github.com/JeremyLoy/config"
)

type RunMode string

const (
	RunModeProd RunMode = "production"
	RunModeDev  RunMode = "development"
)

const (
	DefaultRunMode                   = RunModeDev
	DefaultNamespace                 = "world-1"
	DefaultRedisPassword             = ""
	DefaultRedisAddress              = "localhost:6379"
	DefaultBaseShardSequencerAddress = ""
	DefaultBaseShardQueryAddress     = ""
	DefaultLogLevel      = "info"
	DefaultStatsdEnabled = "localhost:8125"
)

type WorldConfig struct {
	RedisAddress              string  `config:"REDIS_ADDRESS"`
	RedisPassword             string  `config:"REDIS_PASSWORD"`
	CardinalNamespace         string  `config:"CARDINAL_NAMESPACE"`
	CardinalMode              RunMode `config:"CARDINAL_MODE"`
	BaseShardSequencerAddress string  `config:"BASE_SHARD_SEQUENCER_ADDRESS"`
	BaseShardQueryAddress     string  `config:"BASE_SHARD_QUERY_ADDRESS"`
	CardinalLogLevel  string `config:"CARDINAL_LOG_LEVEL"`
	StatsdAddress     string `config:"STATSD_ADDRESS"`
}

var defaultConfig = WorldConfig{
	RedisAddress:              DefaultRedisAddress,
	RedisPassword:             DefaultRedisPassword,
	CardinalNamespace:         DefaultNamespace,
	CardinalMode:              DefaultRunMode,
	BaseShardSequencerAddress: DefaultBaseShardSequencerAddress,
	BaseShardQueryAddress:     DefaultBaseShardQueryAddress,
	CardinalLogLevel: DefaultLogLevel,
	StatsdAddress: DefaultStatsdEnabled,
}

func getWorldConfig() WorldConfig {
	cfg := defaultConfig
	err := config.FromEnv().To(&cfg)
	if err != nil {
		panic(err)
	}
	return cfg
}
