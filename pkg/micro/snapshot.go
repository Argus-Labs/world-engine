package micro

import (
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// Snapshot represents a point-in-time capture of shard state.
// This is an alias to the protobuf-generated type for better API ergonomics.
type Snapshot = microv1.Snapshot

// SnapshotStorage provides persistence for shard snapshots.
// Implementations handle atomic storage with automatic backup of previous snapshots.
type SnapshotStorage interface {
	// Store saves the snapshot, atomically replacing any existing snapshot.
	// The previous snapshot should be preserved as backup if possible.
	Store(snapshot *Snapshot) error

	// Load retrieves the current snapshot.
	// Returns an error if no snapshot exists.
	Load() (*Snapshot, error)

	// Exists checks if a current snapshot is available.
	Exists() bool
}

// SnapshotStorageType defines the type of snapshot storage to use.
type SnapshotStorageType uint8

const (
	SnapshotStorageUndefined SnapshotStorageType = iota
	SnapshotStorageNop
	SnapshotStorageJetStream
)

const (
	nopSnapshotStorageString       = "NOP"
	jetStreamSnapshotStorageString = "JETSTREAM"
	undefinedSnapshotStorageString = "UNDEFINED"
)

func (s SnapshotStorageType) String() string {
	switch s {
	case SnapshotStorageUndefined:
		return undefinedSnapshotStorageString
	case SnapshotStorageNop:
		return nopSnapshotStorageString
	case SnapshotStorageJetStream:
		return jetStreamSnapshotStorageString
	default:
		return undefinedSnapshotStorageString
	}
}

type SnapshotStorageOptions interface {
	validate() error
	apply(ShardOptions)
}
