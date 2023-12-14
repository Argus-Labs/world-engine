package cardinal

import (
	"github.com/JeremyLoy/config"
)

type RunMode string

const (
	DeployModeProd RunMode = "production"
	DeployModeDev  RunMode = "development"
)

const (
	DefaultMode                      = DeployModeDev
	DefaultNamespace                 = "world-1"
	DefaultRedisPassword             = ""
	DefaultRedisAddress              = "localhost:6379"
	DefaultBaseShardSequencerAddress = ""
	DefaultBaseShardQueryAddress     = ""
)

type WorldConfig struct {
	RedisAddress              string  `config:"REDIS_ADDRESS"`
	RedisPassword             string  `config:"REDIS_PASSWORD"`
	CardinalNamespace         string  `config:"CARDINAL_NAMESPACE"`
	CardinalMode              RunMode `config:"CARDINAL_MODE"`
	BaseShardSequencerAddress string  `config:"BASE_SHARD_SEQUENCER_ADDRESS"`
	BaseShardQueryAddress     string  `config:"BASE_SHARD_QUERY_ADDRESS"`
}

var defaultConfig = WorldConfig{
	RedisAddress:              DefaultRedisAddress,
	RedisPassword:             DefaultRedisPassword,
	CardinalNamespace:         DefaultNamespace,
	CardinalMode:              DefaultMode,
	BaseShardSequencerAddress: DefaultBaseShardSequencerAddress,
	BaseShardQueryAddress:     DefaultBaseShardQueryAddress,
}

func GetWorldConfig() WorldConfig {
	cfg := defaultConfig
	err := config.FromEnv().To(&cfg)
	if err != nil {
		panic(err)
	}
	return cfg
}
