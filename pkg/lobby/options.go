package lobby

import (
	"time"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/pkg/micro"
)

// WorldOptions configures the lobby world.
type WorldOptions struct {
	// Shard identity
	Region       string
	Organization string
	Project      string
	ShardID      string

	// Shard behavior
	TickRate       float64
	EpochFrequency uint32

	// Snapshot storage
	SnapshotStorageType    micro.SnapshotStorageType
	SnapshotStorageOptions micro.SnapshotStorageOptions

	// Lobby behavior
	HeartbeatTimeout time.Duration // How long without heartbeat before marking zombie
}

func newDefaultWorldOptions() WorldOptions {
	return WorldOptions{
		Region:              "local",
		Organization:        "",
		Project:             "",
		ShardID:             "lobby",
		TickRate:            1, // Lobby can run at slower tick rate
		EpochFrequency:      10,
		SnapshotStorageType: micro.SnapshotStorageNop,
		HeartbeatTimeout:    15 * time.Minute, // 3 consecutive 5-minute misses
	}
}

func (w *WorldOptions) applyConfig(cfg config) error {
	if cfg.Region != "" {
		w.Region = cfg.Region
	}
	if cfg.Organization != "" {
		w.Organization = cfg.Organization
	}
	if cfg.Project != "" {
		w.Project = cfg.Project
	}
	if cfg.ShardID != "" {
		w.ShardID = cfg.ShardID
	}
	if cfg.TickRate != 0 {
		w.TickRate = float64(cfg.TickRate)
	}
	if cfg.EpochFrequency != 0 {
		w.EpochFrequency = uint32(cfg.EpochFrequency)
	}
	if cfg.SnapshotStorageType != "" {
		switch cfg.SnapshotStorageType {
		case "NOP", "nop":
			w.SnapshotStorageType = micro.SnapshotStorageNop
		case "JETSTREAM", "jetstream":
			w.SnapshotStorageType = micro.SnapshotStorageJetStream
		default:
			return eris.Errorf("invalid snapshot storage type: %s", cfg.SnapshotStorageType)
		}
	}
	if cfg.HeartbeatTimeoutSeconds != 0 {
		w.HeartbeatTimeout = time.Duration(cfg.HeartbeatTimeoutSeconds) * time.Second
	}
	return nil
}

func (w *WorldOptions) apply(opts WorldOptions) {
	if opts.Region != "" {
		w.Region = opts.Region
	}
	if opts.Organization != "" {
		w.Organization = opts.Organization
	}
	if opts.Project != "" {
		w.Project = opts.Project
	}
	if opts.ShardID != "" {
		w.ShardID = opts.ShardID
	}
	if opts.TickRate != 0 {
		w.TickRate = opts.TickRate
	}
	if opts.EpochFrequency != 0 {
		w.EpochFrequency = opts.EpochFrequency
	}
	if opts.SnapshotStorageType != micro.SnapshotStorageUndefined {
		w.SnapshotStorageType = opts.SnapshotStorageType
	}
	if opts.SnapshotStorageOptions != nil {
		w.SnapshotStorageOptions = opts.SnapshotStorageOptions
	}
	if opts.HeartbeatTimeout != 0 {
		w.HeartbeatTimeout = opts.HeartbeatTimeout
	}
}

func (w *WorldOptions) validate() error {
	if w.Organization == "" {
		return eris.New("organization is required")
	}
	if w.Project == "" {
		return eris.New("project is required")
	}
	if w.TickRate <= 0 {
		return eris.New("tick rate must be positive")
	}
	if w.EpochFrequency == 0 {
		return eris.New("epoch frequency must be positive")
	}
	return nil
}

// WithRegion sets the region for the lobby shard.
func WithRegion(region string) func(*WorldOptions) {
	return func(w *WorldOptions) {
		w.Region = region
	}
}

// WithOrganization sets the organization for the lobby shard.
func WithOrganization(org string) func(*WorldOptions) {
	return func(w *WorldOptions) {
		w.Organization = org
	}
}

// WithProject sets the project for the lobby shard.
func WithProject(project string) func(*WorldOptions) {
	return func(w *WorldOptions) {
		w.Project = project
	}
}

// WithShardID sets the shard ID for the lobby shard.
func WithShardID(id string) func(*WorldOptions) {
	return func(w *WorldOptions) {
		w.ShardID = id
	}
}

// WithTickRate sets the tick rate for the lobby shard.
func WithTickRate(rate float64) func(*WorldOptions) {
	return func(w *WorldOptions) {
		w.TickRate = rate
	}
}

// WithEpochFrequency sets the epoch frequency for the lobby shard.
func WithEpochFrequency(freq uint32) func(*WorldOptions) {
	return func(w *WorldOptions) {
		w.EpochFrequency = freq
	}
}

// WithHeartbeatTimeout sets how long without heartbeat before marking zombie.
func WithHeartbeatTimeout(d time.Duration) func(*WorldOptions) {
	return func(w *WorldOptions) {
		w.HeartbeatTimeout = d
	}
}

