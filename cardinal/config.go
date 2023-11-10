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
	RedisAddress       string
	RedisPassword      string
	CardinalNamespace  string
	CardinalPort       string
	CardinalDeployMode string
}

func GetWorldConfig() WorldConfig {
	return WorldConfig{
		RedisAddress:       getEnv("REDIS_ADDRESS", "localhost:6379"),
		RedisPassword:      getEnv("REDIS_PASSWORD", DefaultRedisPassword),
		CardinalNamespace:  getEnv("CARDINAL_NAMESPACE", DefaultCardinalNamespace),
		CardinalPort:       getEnv("CARDINAL_PORT", "4040"),
		CardinalDeployMode: getEnv("CARDINAL_DEPLOY_MODE", CardinalModeDev),
	}
}

func getEnv(key string, fallback string) string {
	var value string
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	if key == "CARDINAL_DEPLOY_MODE" && value != CardinalModeProd && value != CardinalModeDev {
		log.Logger.Warn().
			Msg("CARDINAL_DEPLOY_MODE is not set to [production/development]. Defaulting to development mode.")
		return CardinalModeDev
	}

	return fallback
}
