package cardinal

import (
	"os"

	"github.com/rs/zerolog/log"
)

const (
	CardinalModeProd         = "production"
	CardinalModeDev          = "development"
	DefaultCardinalNamespace = "world-1"
	DefaultRedisPassword     = ""
)

type WorldConfig struct {
	RedisAddress      string
	RedisPassword     string
	CardinalNamespace string
	CardinalPort      string
	CardinalMode      string
}

func GetWorldConfig() WorldConfig {
	return WorldConfig{
		RedisAddress:      getEnv("REDIS_ADDRESS", "localhost:6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", DefaultRedisPassword),
		CardinalNamespace: getEnv("CARDINAL_NAMESPACE", DefaultCardinalNamespace),
		CardinalPort:      getEnv("CARDINAL_PORT", "4040"),
		CardinalMode:      getEnv("CARDINAL_MODE", CardinalModeDev),
	}
}

func getEnv(key string, fallback string) string {
	value, ok := os.LookupEnv(key)
	if ok {
		// Validate CARDINAL_DEPLOY_MODE
		if key == "CARDINAL_MODE" && value != CardinalModeProd && value != CardinalModeDev {
			log.Logger.Warn().
				Msg("CARDINAL_DEPLOY_MODE is not set to [production/development]. Defaulting to development mode.")
			return CardinalModeDev
		}
		return value
	}

	return fallback
}
