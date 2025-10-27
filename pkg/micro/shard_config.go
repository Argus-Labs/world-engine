package micro

import (
	"slices"
	"strings"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/caarlos0/env/v11"
	"github.com/rotisserie/eris"
)

// ShardMode defines the operational mode of a shard instance.
type ShardMode uint8

const (
	ModeUndefined ShardMode = iota // Used as the zero value
	ModeLeader                     // Leader mode processes input and publishes epochs
	ModeFollower                   // Follower mode consumes epochs and replays state
)

const (
	leaderModeString    = "LEADER"
	followerModeString  = "FOLLOWER"
	undefinedModeString = "UNDEFINED"
)

func (m ShardMode) String() string {
	switch m {
	case ModeUndefined:
		return undefinedModeString
	case ModeLeader:
		return leaderModeString
	case ModeFollower:
		return followerModeString
	default:
		return undefinedModeString
	}
}

// shardConfig holds the configuration for a shard instance.
// Configuration can be set via environment variables with the specified defaults.
type shardConfig struct {
	// Shard mode configuration ("LEADER" or "FOLLOWER").
	ModeStr string `env:"SHARD_MODE" envDefault:"LEADER"`

	SnapshotStorageType string `env:"SHARD_SNAPSHOT_STORAGE_TYPE" envDefault:"nop"`

	SnapshotFrequency uint32 `env:"SHARD_SNAPSHOT_FREQUENCY" envDefault:"5"`

	DisablePersona bool `env:"SHARD_DISABLE_PERSONA" envDefault:"false"`

	// Maximum bytes for epoch stream. Required by some NATS providers like Synadia Cloud.
	EpochStreamMaxBytes uint32 `env:"SHARD_EPOCH_STREAM_MAX_BYTES" envDefault:"0"`
}

// loadShardConfig loads the shard configuration from environment variables.
func loadShardConfig() (shardConfig, error) {
	cfg := shardConfig{}

	if err := env.Parse(&cfg); err != nil {
		return cfg, eris.Wrap(err, "failed to parse shard options")
	}

	if err := cfg.validate(); err != nil {
		return cfg, eris.Wrap(err, "failed to validate config")
	}

	return cfg, nil
}

// validate performs validation on the loaded configuration.
func (cfg *shardConfig) validate() error {
	cfg.ModeStr = strings.ToUpper(cfg.ModeStr)
	validModes := []string{"LEADER", "FOLLOWER"}
	if !slices.Contains(validModes, cfg.ModeStr) {
		return eris.Errorf("invalid world mode: %s (must be one of %v)", cfg.ModeStr, validModes)
	}

	cfg.SnapshotStorageType = strings.ToUpper(cfg.SnapshotStorageType)
	validStorageTypes := []string{"NOP", "JETSTREAM"}
	if !slices.Contains(validStorageTypes, cfg.SnapshotStorageType) {
		return eris.Errorf("invalid snapshot storage type: %s (must be one of %v)",
			cfg.SnapshotStorageType, validStorageTypes)
	}

	if cfg.SnapshotFrequency == 0 {
		return eris.New("snapshot frequency cannot be 0")
	}

	// A EpochStreamMaxBytes value of 0 means unlimited epoch stream storage. This is the default, we
	// don't need to validate it here.

	return nil
}

// applyToOptions applies the configuration values to the given ShardOptions.
func (cfg *shardConfig) applyToOptions(opt *ShardOptions) {
	var mode ShardMode
	switch cfg.ModeStr {
	case leaderModeString:
		mode = ModeLeader
	case followerModeString:
		mode = ModeFollower
	default:
		assert.That(true, "unreachable")
	}

	opt.Mode = mode
	opt.SnapshotFrequency = cfg.SnapshotFrequency
	opt.DisablePersona = cfg.DisablePersona
	opt.EpochStreamMaxBytes = cfg.EpochStreamMaxBytes
}

const MinEpochFrequency = 10

// ShardOptions contains configuration options for creating a new shard.
type ShardOptions struct {
	Client                 *Client                // NATS client
	Address                *ServiceAddress        // Shard's service address
	Mode                   ShardMode              // Operation mode (Leader or Follower)
	EpochFrequency         uint32                 // Number of ticks per epoch
	TickRate               float64                // Number of ticks per second
	Telemetry              *telemetry.Telemetry   // Telemetry for logging and tracing
	SnapshotStorageType    SnapshotStorageType    // Snapshot storage type
	SnapshotStorageOptions SnapshotStorageOptions // Optional snapshot storage options
	SnapshotFrequency      uint32                 // Number of epochs per snapshot
	DisablePersona         bool                   // Disable persona verification for development/testing
	EpochStreamMaxBytes    uint32                 // Maximum bytes for epoch stream (required by some NATS providers)
}

// newDefaultShardOptions creates ShardOptions with default values.
func newDefaultShardOptions() ShardOptions {
	// Set these to invalid values to force users to pass in the correct options.
	return ShardOptions{
		Client:                 nil,
		Address:                nil,
		Mode:                   ModeLeader,
		EpochFrequency:         0,
		TickRate:               0,
		Telemetry:              nil,
		SnapshotStorageType:    SnapshotStorageUndefined,
		SnapshotStorageOptions: nil,
		SnapshotFrequency:      0,
		EpochStreamMaxBytes:    0, // There is no invalid values for this, just set default of 0
	}
}

// apply merges the given options into the current options, overriding non-zero values.
func (opt *ShardOptions) apply(newOpt ShardOptions) {
	if newOpt.Client != nil {
		opt.Client = newOpt.Client
	}
	if newOpt.Address != nil {
		opt.Address = newOpt.Address
	}
	if newOpt.Telemetry != nil {
		opt.Telemetry = newOpt.Telemetry
	}
	if newOpt.Mode != ModeUndefined {
		opt.Mode = newOpt.Mode
	}
	if newOpt.EpochFrequency != 0 {
		opt.EpochFrequency = newOpt.EpochFrequency
	}
	if newOpt.TickRate != 0.0 {
		opt.TickRate = newOpt.TickRate
	}
	if newOpt.SnapshotStorageType != SnapshotStorageUndefined {
		opt.SnapshotStorageType = newOpt.SnapshotStorageType
	}
	if newOpt.SnapshotStorageOptions != nil {
		opt.SnapshotStorageOptions = newOpt.SnapshotStorageOptions
	}
	if newOpt.SnapshotFrequency != 0 {
		opt.SnapshotFrequency = newOpt.SnapshotFrequency
	}
	// These options' zero values are always valid, so if unset they will always override opt.
	// We'll just make them configurable only from the env var.
	// These include: DisablePersona, EpochStreamMaxBytes
}

// validate checks that all required options are set and valid.
func (opt *ShardOptions) validate() error {
	if opt.Client == nil {
		return eris.New("NATS client cannot be nil")
	}
	if opt.Address == nil {
		return eris.New("service address cannot be nil")
	}
	if opt.Telemetry == nil {
		return eris.New("telemetry cannot be nil")
	}
	if opt.EpochFrequency < MinEpochFrequency {
		return eris.Errorf("epoch frequency must be at least %d", MinEpochFrequency)
	}
	if opt.TickRate == 0.0 {
		return eris.New("tick rate cannot be 0")
	}

	// Mode validation.
	switch opt.Mode {
	case ModeUndefined:
		return eris.New("shard mode must be specified")
	case ModeFollower, ModeLeader:
		// Valid modes.
	}

	// Snapshot storage validation.
	if opt.SnapshotStorageType == SnapshotStorageUndefined {
		return eris.New("snapshot storage type must be specified")
	}
	// SnapshotStorageOptions can be nil.

	// Snapshot frequency validation.
	if opt.SnapshotFrequency == 0 {
		return eris.New("snapshot frequency cannot be 0")
	}

	return nil
}
