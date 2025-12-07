package matchmaking

import (
	"time"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/pkg/matchmaking/store"
	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
	"github.com/argus-labs/world-engine/pkg/micro"
)

// WorldOptions contains configuration options for the matchmaking world.
type WorldOptions struct {
	// Region is the geographic region identifier (e.g., "us-west-2").
	Region string

	// Organization is the entity that owns the project.
	Organization string

	// Project is the project name.
	Project string

	// ShardID is the unique identifier for this shard instance.
	ShardID string

	// TickRate is the number of ticks per second.
	TickRate float64

	// EpochFrequency is the number of ticks per epoch.
	EpochFrequency uint32

	// SnapshotStorageType specifies how snapshots are stored.
	SnapshotStorageType micro.SnapshotStorageType

	// SnapshotStorageOptions contains additional options for snapshot storage.
	SnapshotStorageOptions micro.SnapshotStorageOptions

	// MatchProfiles is the list of match profiles to use.
	// If empty, profiles will be loaded from the config file.
	MatchProfiles []*types.Profile

	// MatchProfilesJSON is the raw JSON data for match profiles.
	// Used when profiles are provided programmatically.
	MatchProfilesJSON []byte

	// BackfillTTL is the default time-to-live for backfill requests.
	// If zero, defaults to 5 minutes.
	// Note: Ticket TTL is specified per-ticket by the Game Shard (defaults to 1 hour).
	BackfillTTL time.Duration
}

// worldOptionsInternal holds the fully resolved options.
type worldOptionsInternal struct {
	Region                 string
	Organization           string
	Project                string
	ShardID                string
	TickRate               float64
	EpochFrequency         uint32
	SnapshotStorageType    micro.SnapshotStorageType
	SnapshotStorageOptions micro.SnapshotStorageOptions
	MatchProfiles          *store.ProfileStore
	BackfillTTL            time.Duration
}

// newDefaultWorldOptions creates options with sensible defaults.
func newDefaultWorldOptions() worldOptionsInternal {
	return worldOptionsInternal{
		Region:              "local",
		Organization:        "default",
		Project:             "default",
		ShardID:             "matchmaking-1",
		TickRate:            10,
		EpochFrequency:      100,
		SnapshotStorageType: micro.SnapshotStorageNop,
		MatchProfiles:       store.NewProfileStore(),
		BackfillTTL:         5 * time.Minute,
	}
}

// apply applies user-provided options to the internal options.
func (o *worldOptionsInternal) apply(opts WorldOptions) {
	if opts.Region != "" {
		o.Region = opts.Region
	}
	if opts.Organization != "" {
		o.Organization = opts.Organization
	}
	if opts.Project != "" {
		o.Project = opts.Project
	}
	if opts.ShardID != "" {
		o.ShardID = opts.ShardID
	}
	if opts.TickRate > 0 {
		o.TickRate = opts.TickRate
	}
	if opts.EpochFrequency > 0 {
		o.EpochFrequency = opts.EpochFrequency
	}
	if opts.SnapshotStorageType != micro.SnapshotStorageUndefined {
		o.SnapshotStorageType = opts.SnapshotStorageType
	}
	if opts.SnapshotStorageOptions != nil {
		o.SnapshotStorageOptions = opts.SnapshotStorageOptions
	}
	if opts.BackfillTTL > 0 {
		o.BackfillTTL = opts.BackfillTTL
	}
}

// applyConfig applies environment-based config to the options.
func (o *worldOptionsInternal) applyConfig(cfg config) error {
	if cfg.BackfillTTLSeconds > 0 {
		o.BackfillTTL = cfg.BackfillTTL()
	}
	return nil
}

// loadMatchProfiles loads match profiles from various sources.
func (o *worldOptionsInternal) loadMatchProfiles(opts WorldOptions, cfg config) error {
	// Priority: explicit profiles > JSON bytes > config file path

	// If explicit profiles provided, use them
	if len(opts.MatchProfiles) > 0 {
		for _, p := range opts.MatchProfiles {
			if err := o.MatchProfiles.Add(p); err != nil {
				return err
			}
		}
		return nil
	}

	// If JSON bytes provided, parse them
	if len(opts.MatchProfilesJSON) > 0 {
		profileStore, err := store.LoadProfilesFromJSON(opts.MatchProfilesJSON)
		if err != nil {
			return eris.Wrap(err, "failed to load match profiles from JSON")
		}
		o.MatchProfiles = profileStore
		return nil
	}

	// If config file path provided, load from file
	if cfg.MatchProfilesPath != "" {
		profileStore, err := store.LoadProfilesFromFile(cfg.MatchProfilesPath)
		if err != nil {
			return eris.Wrap(err, "failed to load match profiles from file")
		}
		o.MatchProfiles = profileStore
		return nil
	}

	// No profiles configured - this is okay, they can be added later
	return nil
}

// validate validates the options.
func (o *worldOptionsInternal) validate() error {
	if o.Region == "" {
		return eris.New("region is required")
	}
	if o.Organization == "" {
		return eris.New("organization is required")
	}
	if o.Project == "" {
		return eris.New("project is required")
	}
	if o.ShardID == "" {
		return eris.New("shard_id is required")
	}
	if o.TickRate <= 0 {
		return eris.New("tick_rate must be positive")
	}
	if o.EpochFrequency == 0 {
		return eris.New("epoch_frequency must be positive")
	}
	if o.BackfillTTL <= 0 {
		return eris.New("backfill_ttl must be positive")
	}
	return nil
}
