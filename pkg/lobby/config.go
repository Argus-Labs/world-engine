package lobby

import (
	"github.com/caarlos0/env/v11"
	"github.com/rotisserie/eris"
)

// config holds environment variable configuration for the lobby shard.
type config struct {
	// Shard identity
	Region       string `env:"REGION"`
	Organization string `env:"ORGANIZATION"`
	Project      string `env:"PROJECT"`
	ShardID      string `env:"SHARD_ID"`

	// Shard behavior
	TickRate       int `env:"TICK_RATE"`
	EpochFrequency int `env:"EPOCH_FREQUENCY"`

	// Snapshot storage
	SnapshotStorageType string `env:"SHARD_SNAPSHOT_STORAGE_TYPE"`

	// Lobby behavior
	HeartbeatTimeoutSeconds int `env:"HEARTBEAT_TIMEOUT_SECONDS"`
}

func loadConfig() (config, error) {
	cfg := config{}
	if err := env.Parse(&cfg); err != nil {
		return cfg, eris.Wrap(err, "failed to parse environment variables")
	}
	return cfg, nil
}
