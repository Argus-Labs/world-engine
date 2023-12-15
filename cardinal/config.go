package cardinal

import (
	"os"

	"github.com/rs/zerolog/log"
)

const (
	ModeProd             = "production"
	ModeDev              = "development"
	DefaultMode          = ModeDev
	DefaultNamespace     = "world-1"
	DefaultRedisPassword = ""
	DefaultRedisAddress  = "localhost:6379"
	DefaultLogLevel      = "info"
)

type WorldConfig struct {
	RedisAddress      string
	RedisPassword     string
	CardinalNamespace string
	CardinalPort      string
	CardinalMode      string
	CardinalLogLevel  string
}

func GetWorldConfig() WorldConfig {
	return WorldConfig{
		RedisAddress:      getEnv("REDIS_ADDRESS", DefaultRedisAddress),
		RedisPassword:     getEnv("REDIS_PASSWORD", DefaultRedisPassword),
		CardinalNamespace: getEnv("CARDINAL_NAMESPACE", DefaultNamespace),
		CardinalPort:      getEnv("CARDINAL_PORT", "4040"),
		CardinalMode:      getEnv("CARDINAL_MODE", DefaultMode),
		CardinalLogLevel:  getEnv("CARDINAL_LOG_LEVEL", DefaultLogLevel),
	}
}

func getEnv(key string, fallback string) string {
	value, ok := os.LookupEnv(key)
	if ok {
		// Validate CARDINAL_DEPLOY_MODE
		if key == "CARDINAL_MODE" && value != ModeProd && value != ModeDev {
			log.Logger.Warn().
				Msg("CARDINAL_DEPLOY_MODE is not set to [production/development]. Defaulting to development mode.")
			return ModeDev
		}
		return value
	}

	return fallback
}
