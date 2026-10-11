package cardinal

import (
	"time"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/caarlos0/env/v11"
	"github.com/rotisserie/eris"
)

const MinEpochFrequency = 10

type WorldOptions struct {
	Project             string               // Project that groups related shards
	ShardID             string               // Unique ID of the world instance within the project
	TickRate            float64              // Number of ticks each second
	SnapshotStorageType snapshot.StorageType // Snapshot storage type
	SnapshotRate        uint32               // Number of ticks between snapshots
	Debug               *bool                // Enables the debug server
	NATSConfig          *micro.NATSConfig    // Optional NATS configuration; nil uses environment values or defaults
	AuthMode            AuthMode             // Authentication mode for the client ConnectRPC service
	ArgusAuthURL        string               // Argus Auth service URL; required when AuthMode is ARGUS
	ArgusAuthGameID     string               // Argus Auth game ID tokens are issued for; required when AuthMode is ARGUS
}

// tickInterval is the time between ticks, or zero when no tick rate is set (as in TestWorld).
func (opt *WorldOptions) tickInterval() time.Duration {
	if opt.TickRate <= 0 {
		return 0
	}
	return time.Duration(float64(time.Second) / opt.TickRate)
}

// newDefaultWorldOptions creates WorldOptions with default values.
func newDefaultWorldOptions() WorldOptions {
	// Set defaults for optional fields. Set invalid values for fields that callers must provide.
	return WorldOptions{
		Project:             "",
		ShardID:             "",
		TickRate:            0,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        0,
		Debug:               nil,
		AuthMode:            AuthModeDev,
		ArgusAuthURL:        "",
		ArgusAuthGameID:     "",
	}
}

// apply replaces each option that has a nonzero value in newOpt.
func (opt *WorldOptions) apply(newOpt WorldOptions) {
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
	if newOpt.AuthMode.IsValid() {
		opt.AuthMode = newOpt.AuthMode
	}
	if newOpt.ArgusAuthURL != "" {
		opt.ArgusAuthURL = newOpt.ArgusAuthURL
	}
	if newOpt.ArgusAuthGameID != "" {
		opt.ArgusAuthGameID = newOpt.ArgusAuthGameID
	}
}

// validate checks that all required options have valid values.
func (opt *WorldOptions) validate() error {
	if opt.Project == "" {
		return eris.New("project cannot be empty")
	}
	if opt.ShardID == "" {
		return eris.New("shard ID cannot be empty")
	}
	if err := protovalidate.Validate(micro.GetAddress(opt.Project, opt.ShardID)); err != nil {
		return eris.Wrap(err, "invalid project or shard ID")
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
	if !opt.AuthMode.IsValid() {
		return eris.Errorf("invalid auth mode: %s (must be one of: ARGUS, DEV)", opt.AuthMode)
	}
	if opt.AuthMode == AuthModeArgus && opt.ArgusAuthURL == "" {
		return eris.New("argus auth URL cannot be empty when auth mode is ARGUS")
	}
	if opt.AuthMode == AuthModeArgus && opt.ArgusAuthGameID == "" {
		return eris.New("argus auth game ID cannot be empty when auth mode is ARGUS")
	}
	return nil
}

func (opt *WorldOptions) getPosthogBaseProperties() map[string]any {
	return map[string]any{
		"project":  opt.Project,
		"shard_id": opt.ShardID,
	}
}

func (opt *WorldOptions) getSentryTags() map[string]string {
	return map[string]string{
		"project":  opt.Project,
		"shard_id": opt.ShardID,
	}
}

// -------------------------------------------------------------------------------------------------
// World options environment variables
// -------------------------------------------------------------------------------------------------

// TODO: Update the environment variables.
// worldOptionsEnv contains WorldOptions values from environment variables.
type worldOptionsEnv struct {
	// Project that groups related shards.
	Project string `env:"CARDINAL_PROJECT"`

	// Unique ID of the world instance within the project.
	ShardID string `env:"CARDINAL_SHARD_ID"`

	// Snapshot storage type ("NOP", "JETSTREAM", or "S3").
	SnapshotStorageTypeStr string `env:"CARDINAL_SNAPSHOT_STORAGE_TYPE" envDefault:"NOP"`

	// Number of ticks between snapshots. See docs/cardinal/snapshots.mdx for the durability trade
	// and what happens when storage is slower than the snapshot rate.
	SnapshotRate uint32 `env:"CARDINAL_SNAPSHOT_RATE"`

	// Enables the debug server.
	Debug bool `env:"CARDINAL_DEBUG" envDefault:"false"`

	// Authentication mode for the client ConnectRPC service: ARGUS or DEV.
	AuthModeStr string `env:"CARDINAL_AUTH_MODE" envDefault:"DEV"`

	// Argus Auth service URL. This value is required when AuthMode is ARGUS.
	ArgusAuthURL string `env:"CARDINAL_ARGUS_AUTH_URL"`

	// Argus Auth game ID that player tokens are issued for (their aud claim, e.g. "argus/rampage").
	// This value is required when AuthMode is ARGUS.
	ArgusAuthGameID string `env:"CARDINAL_ARGUS_AUTH_GAME_ID"`
}

// loadWorldOptionsEnv reads and validates world options from environment variables.
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

// validate checks the environment configuration.
func (cfg *worldOptionsEnv) validate() error {
	if _, err := snapshot.ParseStorageType(cfg.SnapshotStorageTypeStr); err != nil {
		return err
	}
	// ARGUS's URL and game ID may come from WorldOptions instead, so WorldOptions.validate checks
	// them after the merge.
	if _, err := ParseAuthMode(cfg.AuthModeStr); err != nil {
		return eris.Wrap(err, "failed to parse auth mode")
	}
	return nil
}

// toOptions converts the environment configuration to WorldOptions.
func (cfg *worldOptionsEnv) toOptions() WorldOptions {
	snapshotStorageType, err := snapshot.ParseStorageType(cfg.SnapshotStorageTypeStr)
	assert.That(err == nil, "config not validated")

	authMode, err := ParseAuthMode(cfg.AuthModeStr)
	assert.That(err == nil, "config not validated")

	return WorldOptions{
		Project:             cfg.Project,
		ShardID:             cfg.ShardID,
		SnapshotStorageType: snapshotStorageType,
		SnapshotRate:        cfg.SnapshotRate,
		Debug:               &cfg.Debug,
		AuthMode:            authMode,
		ArgusAuthURL:        cfg.ArgusAuthURL,
		ArgusAuthGameID:     cfg.ArgusAuthGameID,
	}
}
