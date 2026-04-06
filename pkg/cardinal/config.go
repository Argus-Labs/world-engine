package cardinal

import (
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/caarlos0/env/v11"
	"github.com/rotisserie/eris"
)

const MinEpochFrequency = 10

type WorldOptions struct {
	Region              string               // Region the shard is deployed to
	Organization        string               // The organization that owns this world
	Project             string               // Name of the project within the organization
	ShardID             string               // Unique ID for of world's instance
	TickRate            float64              // Number of ticks per second
	SnapshotStorageType snapshot.StorageType // Snapshot storage type
	SnapshotRate        uint32               // Number of ticks per snapshot
	Debug               *bool                // Enable debug server
	NATSConfig          *micro.NATSConfig    // Optional NATS config override (nil = use env/defaults)
	DiskStoragePath string // Local disk path for disk-backed components (empty = disabled)

	// Number of ticks between disk compactions. Compaction rewrites the Bitcask file
	// without dead entries (old versions from updates and deletes).
	//
	// Set to 0 to disable periodic compaction. Compaction still runs before every
	// snapshot regardless of this setting, to ensure the snapshot file blob is clean.
	//
	// Formula for choosing a value based on 50% waste ratio threshold:
	//   CompactionRate = total_live_entities / updates_per_tick
	//
	// Example: 10,000 entities, 100 updates per tick -> CompactionRate = 100
	// Example: 10,000 entities, 10 updates per tick -> CompactionRate = 1,000
	//
	// For progression-based systems where data mostly grows and rarely updates,
	// set to 0 and rely on pre-snapshot compaction.
	CompactionRate uint32
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
		TickRate:            0,
		SnapshotStorageType: snapshot.StorageTypeNop, // Default to nop snapshot
		SnapshotRate:        0,
		Debug:               nil,
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
	if newOpt.TickRate > 0.0 {
		opt.TickRate = newOpt.TickRate
	}
	if newOpt.SnapshotStorageType.IsValid() {
		opt.SnapshotStorageType = newOpt.SnapshotStorageType
	}
	if newOpt.SnapshotRate != 0 {
		opt.SnapshotRate = newOpt.SnapshotRate
	}
	if newOpt.Debug != nil {
		opt.Debug = newOpt.Debug
	}
	if newOpt.NATSConfig != nil {
		opt.NATSConfig = newOpt.NATSConfig
	}
	if newOpt.DiskStoragePath != "" {
		opt.DiskStoragePath = newOpt.DiskStoragePath
	}
	if newOpt.CompactionRate != 0 {
		opt.CompactionRate = newOpt.CompactionRate
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
	if opt.TickRate <= 0.0 {
		return eris.New("tick rate must be greater than 0")
	}
	if !opt.SnapshotStorageType.IsValid() {
		return eris.New("snapshot storage type must be specified")
	}
	if opt.SnapshotRate == 0 {
		return eris.New("snapshot frequency cannot be 0")
	}
	if opt.Debug == nil {
		return eris.New("debug must be specified")
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

// TODO: update envs.
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

	// Snapshot storage type ("NOP", "JETSTREAM", or "S3").
	SnapshotStorageTypeStr string `env:"CARDINAL_SNAPSHOT_STORAGE_TYPE" envDefault:"NOP"`

	// Number of ticks per snapshot.
	SnapshotRate uint32 `env:"CARDINAL_SNAPSHOT_RATE"`

	// Enable debug server.
	Debug bool `env:"CARDINAL_DEBUG" envDefault:"false"`

	// Local disk path for disk-backed components (empty = disabled).
	DiskStoragePath string `env:"CARDINAL_DISK_STORAGE_PATH"`

	// Number of ticks per disk compaction (0 = disabled).
	CompactionRate uint32 `env:"CARDINAL_COMPACTION_RATE"`
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
	if _, err := snapshot.ParseStorageType(cfg.SnapshotStorageTypeStr); err != nil {
		return err
	}
	return nil
}

// toOptions converts the worldOptionsEnv to WorldOptions.
func (cfg *worldOptionsEnv) toOptions() WorldOptions {
	snapshotStorageType, err := snapshot.ParseStorageType(cfg.SnapshotStorageTypeStr)
	assert.That(err == nil, "config not validated")

	return WorldOptions{
		Region:              cfg.Region,
		Organization:        cfg.Organization,
		Project:             cfg.Project,
		ShardID:             cfg.ShardID,
		SnapshotStorageType: snapshotStorageType,
		SnapshotRate:        cfg.SnapshotRate,
		Debug:               &cfg.Debug,
		DiskStoragePath:     cfg.DiskStoragePath,
		CompactionRate:      cfg.CompactionRate,
	}
}
