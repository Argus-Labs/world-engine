package cardinal

import "os"

type WorldConfig struct {
	RedisAddress       string
	RedisPassword      string
	CardinalWorldId    string
	CardinalPort       string
	CardinalDeployMode string
}

func GetWorldConfig() WorldConfig {
	return WorldConfig{
		RedisAddress:       getEnv("REDIS_ADDRESS", "localhost:6379"),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		CardinalWorldId:    getEnv("CARDINAL_WORLD_ID", "world"),
		CardinalPort:       getEnv("CARDINAL_PORT", "3333"),
		CardinalDeployMode: getEnv("CARDINAL_DEPLOY_MODE", "development"),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
