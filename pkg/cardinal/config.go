package cardinal

import (
	"strings"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/caarlos0/env/v11"
	"github.com/rotisserie/eris"
)

const MinEpochFrequency = 10

type WorldOptions struct {
	Region              string               // Region the shard is deployed to
	Organization        string               // The organization that owns this world
	Project             string               // Name of the project within the organization
	ShardID             string               // Unique ID for of world's instance
	Mode                ShardMode            // Operation mode (Leader or Follower)
	EpochFrequency      uint32               // Number of ticks per epoch
	TickRate            float64              // Number of ticks per second
	SnapshotStorageType snapshot.StorageType // Snapshot storage type
	SnapshotFrequency   uint32               // Number of epochs per snapshot
	EpochStreamMaxBytes uint32               // Maximum bytes for epoch stream (required by some NATS providers)
}

// newDefaultWorldOptions creates WorldOptions with default values.
func newDefaultWorldOptions() WorldOptions {
	// Initialize optional fields with defaults and initialize required fields with invalid values to
	// force callers to provide them explicitly.
	return WorldOptions{
		Region:              "",
		Organization:        "",
		Project:             "",
		ShardID:             "",
		Mode:                ModeLeader, // Default to leader mode
		EpochFrequency:      0,
		TickRate:            0,
		SnapshotStorageType: snapshot.StorageTypeNop, // Default to nop snapshot
		SnapshotFrequency:   0,
		EpochStreamMaxBytes: 0, // There is no invalid values for this, just set default of 0
	}
}

// apply merges the given options into the current options, overriding non-zero values.
func (opt *WorldOptions) apply(newOpt WorldOptions) {
	if newOpt.Region != "" {
		opt.Region = newOpt.Region
	}
	if newOpt.Organization != "" {
		opt.Organization = newOpt.Organization
	}
	if newOpt.Project != "" {
		opt.Project = newOpt.Project
	}
	if newOpt.ShardID != "" {
		opt.ShardID = newOpt.ShardID
	}
	if newOpt.Mode.IsValid() {
		opt.Mode = newOpt.Mode
	}
	if newOpt.EpochFrequency != 0 {
		opt.EpochFrequency = newOpt.EpochFrequency
	}
	if newOpt.TickRate != 0.0 {
		opt.TickRate = newOpt.TickRate
	}
	if newOpt.SnapshotStorageType.IsValid() {
		opt.SnapshotStorageType = newOpt.SnapshotStorageType
	}
	if newOpt.SnapshotFrequency != 0 {
		opt.SnapshotFrequency = newOpt.SnapshotFrequency
	}
	if newOpt.EpochStreamMaxBytes != 0 {
		opt.EpochStreamMaxBytes = newOpt.EpochStreamMaxBytes
	}
}

// validate checks that all required options are set and valid.
func (opt *WorldOptions) validate() error {
	if opt.Region == "" {
		return eris.New("region cannot be empty")
	}
	if opt.Organization == "" {
		return eris.New("organization cannot be empty")
	}
	if opt.Project == "" {
		return eris.New("project cannot be empty")
	}
	if opt.ShardID == "" {
		return eris.New("shard ID cannot be empty")
	}
	if opt.EpochFrequency < MinEpochFrequency {
		return eris.Errorf("epoch frequency must be at least %d", MinEpochFrequency)
	}
	if opt.TickRate == 0.0 {
		return eris.New("tick rate cannot be 0")
	}
	if !opt.Mode.IsValid() {
		return eris.New("invalid shard mode")
	}
	if !opt.SnapshotStorageType.IsValid() {
		return eris.New("snapshot storage type must be specified")
	}
	if opt.SnapshotFrequency == 0 {
		return eris.New("snapshot frequency cannot be 0")
	}
	return nil
}

func (opt *WorldOptions) getPosthogBaseProperties() map[string]any {
	return map[string]any{
		"region":   opt.Region,
		"project":  opt.Project,
		"shard_id": opt.ShardID,
	}
}

func (opt *WorldOptions) getSentryTags() map[string]string {
	return map[string]string{
		"region":       opt.Region,
		"organization": opt.Organization,
		"project":      opt.Project,
		"shard_id":     opt.ShardID,
	}
}

// -------------------------------------------------------------------------------------------------
// World options environment variables
// -------------------------------------------------------------------------------------------------

// worldOptionsEnv are WorldOption values set through env variables.
type worldOptionsEnv struct {
	// Region the shard is deployed to.
	Region string `env:"CARDINAL_REGION"`

	// The organization that owns this world.
	Organization string `env:"CARDINAL_ORG"`

	// Name of the project within the organization.
	Project string `env:"CARDINAL_PROJECT"`

	// Unique ID of this world's instance.
	ShardID string `env:"CARDINAL_SHARD_ID"`

	// Shard mode configuration ("LEADER" or "FOLLOWER").
	ModeStr string `env:"SHARD_MODE" envDefault:"LEADER"`

	// Snapshot storage type ("NOP" or "JETSTREAM").
	SnapshotStorageTypeStr string `env:"SHARD_SNAPSHOT_STORAGE_TYPE" envDefault:"NOP"`

	// Number of epochs per snapshot.
	SnapshotFrequency uint32 `env:"SHARD_SNAPSHOT_FREQUENCY"`

	// Maximum bytes for epoch stream. Required by some NATS providers like Synadia Cloud.
	EpochStreamMaxBytes uint32 `env:"SHARD_EPOCH_STREAM_MAX_BYTES"`
}

// loadWorldOptionsEnv loads the world options from environment variables.
func loadWorldOptionsEnv() (worldOptionsEnv, error) {
	cfg := worldOptionsEnv{}

	if err := env.Parse(&cfg); err != nil {
		return cfg, eris.Wrap(err, "failed to parse world config")
	}

	if err := cfg.validate(); err != nil {
		return cfg, eris.Wrap(err, "failed to validate config")
	}

	return cfg, nil
}

// validate performs validation on the loaded configuration.
func (cfg *worldOptionsEnv) validate() error {
	if _, err := parseShardMode(cfg.ModeStr); err != nil {
		return err
	}
	if _, err := snapshot.ParseStorageType(cfg.SnapshotStorageTypeStr); err != nil {
		return err
	}
	return nil
}

// toOptions converts the worldOptionsEnv to WorldOptions.
func (cfg *worldOptionsEnv) toOptions() WorldOptions {
	mode, err := parseShardMode(cfg.ModeStr)
	assert.That(err == nil, "config not validated")

	snapshotStorageType, err := snapshot.ParseStorageType(cfg.SnapshotStorageTypeStr)
	assert.That(err == nil, "config not validated")

	return WorldOptions{
		Region:              cfg.Region,
		Organization:        cfg.Organization,
		Project:             cfg.Project,
		ShardID:             cfg.ShardID,
		Mode:                mode,
		SnapshotStorageType: snapshotStorageType,
		SnapshotFrequency:   cfg.SnapshotFrequency,
		EpochStreamMaxBytes: cfg.EpochStreamMaxBytes,
	}
}

// -------------------------------------------------------------------------------------------------
// Shard mode
// -------------------------------------------------------------------------------------------------

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

func (m ShardMode) IsValid() bool {
	return m == ModeLeader || m == ModeFollower
}

func parseShardMode(s string) (ShardMode, error) {
	switch strings.ToUpper(s) {
	case leaderModeString:
		return ModeLeader, nil
	case followerModeString:
		return ModeFollower, nil
	default:
		return ModeUndefined, eris.Errorf("invalid shard mode: %s (must be LEADER or FOLLOWER)", s)
	}
}
