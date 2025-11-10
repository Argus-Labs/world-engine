package cardinal

import (
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/caarlos0/env/v11"
	"github.com/rotisserie/eris"
)

// worldConfig holds the configuration for a Cardinal World instance.
// Configuration can be set via environment variables with the specified defaults.
type worldConfig struct {
	// Region the shard is deployed to.
	Region string `env:"CARDINAL_REGION"`

	// The organization that owns this world.
	Organization string `env:"CARDINAL_ORG" envDefault:"organization"`

	// Name of the project within the organization.
	Project string `env:"CARDINAL_PROJECT" envDefault:"project"`

	// Unique ID of this world's instance.
	ShardID string `env:"CARDINAL_SHARD_ID" envDefault:"service"`

	// Hex-encoded Ed25519 private key used for signing (inter-shard) commands.
	PrivateKey string `env:"CARDINAL_PRIVATE_KEY"`
}

// loadWorldConfig loads the world configuration from environment variables.
func loadWorldConfig() (worldConfig, error) {
	cfg := worldConfig{}

	if err := env.Parse(&cfg); err != nil {
		return cfg, eris.Wrap(err, "failed to parse world config")
	}

	if err := cfg.validate(); err != nil {
		return cfg, eris.Wrap(err, "failed to validate config")
	}

	return cfg, nil
}

// validate performs validation on the loaded configuration.
func (cfg *worldConfig) validate() error {
	if cfg.Region == "" {
		return eris.New("region cannot be empty")
	}
	if cfg.Organization == "" {
		return eris.New("organization cannot be empty")
	}
	if cfg.Project == "" {
		return eris.New("project cannot be empty")
	}
	if cfg.ShardID == "" {
		return eris.New("shard ID cannot be empty")
	}
	if cfg.PrivateKey == "" {
		return eris.New("private key cannot be empty")
	}

	return nil
}

// applyToOptions applies the configuration values to the given ShardOptions.
func (cfg *worldConfig) applyToOptions(opt *WorldOptions) {
	opt.Region = cfg.Region
	opt.Organization = cfg.Organization
	opt.Project = cfg.Project
	opt.ShardID = cfg.ShardID
	opt.PrivateKey = cfg.PrivateKey
}

type WorldOptions struct {
	Region                 string                       // Region the shard is deployed to
	Organization           string                       // The organization that owns this world
	Project                string                       // Name of the project within the organization
	ShardID                string                       // Unique ID for of world's instance
	PrivateKey             string                       // Hex-encoded Ed25519 private key for signing commands
	EpochFrequency         uint32                       // Number of ticks per epoch
	TickRate               float64                      // Number of ticks per second
	SnapshotStorageType    micro.SnapshotStorageType    // Snapshot storage type
	SnapshotStorageOptions micro.SnapshotStorageOptions // Optional snapshot storage options
}

// newDefaultWorldOptions creates WorldOptions with default values.
func newDefaultWorldOptions() WorldOptions {
	// Set these to invalid values to force users to pass in the correct options.
	return WorldOptions{
		Region:                 "",
		Organization:           "",
		Project:                "",
		ShardID:                "",
		PrivateKey:             "",
		EpochFrequency:         0,
		TickRate:               0,
		SnapshotStorageType:    micro.SnapshotStorageNop, // Default to nop snapshot
		SnapshotStorageOptions: nil,
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
	if newOpt.PrivateKey != "" {
		opt.PrivateKey = newOpt.PrivateKey
	}
	if newOpt.EpochFrequency != 0 {
		opt.EpochFrequency = newOpt.EpochFrequency
	}
	if newOpt.TickRate != 0.0 {
		opt.TickRate = newOpt.TickRate
	}
	if newOpt.SnapshotStorageType != micro.SnapshotStorageUndefined {
		opt.SnapshotStorageType = newOpt.SnapshotStorageType
	}
	if newOpt.SnapshotStorageOptions != nil {
		opt.SnapshotStorageOptions = newOpt.SnapshotStorageOptions
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
	if opt.PrivateKey == "" {
		return eris.New("private key cannot be empty")
	}
	if opt.EpochFrequency < micro.MinEpochFrequency {
		return eris.Errorf("epoch frequency must be at least %d", micro.MinEpochFrequency)
	}
	if opt.TickRate == 0.0 {
		return eris.New("tick rate cannot be 0")
	}
	if opt.SnapshotStorageType == micro.SnapshotStorageUndefined {
		return eris.New("invalid snapshot storage type")
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
