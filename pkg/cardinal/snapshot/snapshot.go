package snapshot

import (
	"strings"
	"time"

	"github.com/rotisserie/eris"
)

// Snapshot represents a point-in-time capture of shard state.
// This is an alias to the protobuf-generated type for better API ergonomics.
type Snapshot struct {
	EpochHeight uint64
	TickHeight  uint64
	Timestamp   time.Time
	StateHash   []byte
	Data        []byte
}

// Storage provides persistence for shard snapshots.
// Implementations handle atomic storage with automatic backup of previous snapshots.
type Storage interface {
	// Store saves the snapshot, atomically replacing any existing snapshot.
	// The previous snapshot should be preserved as backup if possible.
	Store(snapshot *Snapshot) error

	// Load retrieves the current snapshot.
	// Returns an error if no snapshot exists.
	Load() (*Snapshot, error)

	// Exists checks if a current snapshot is available.
	Exists() bool
}

// StorageType defines the type of snapshot storage to use.
type StorageType uint8

const (
	StorageTypeUndefined StorageType = iota
	StorageTypeNop
	StorageTypeJetStream
)

const (
	nopStorageString       = "NOP"
	jetStreamStorageString = "JETSTREAM"
	undefinedStorageString = "UNDEFINED"
)

func (s StorageType) String() string {
	switch s {
	case StorageTypeUndefined:
		return undefinedStorageString
	case StorageTypeNop:
		return nopStorageString
	case StorageTypeJetStream:
		return jetStreamStorageString
	default:
		return undefinedStorageString
	}
}

func (m StorageType) IsValid() bool {
	return m == StorageTypeNop || m == StorageTypeJetStream
}

func ParseStorageType(s string) (StorageType, error) {
	switch strings.ToUpper(s) {
	case nopStorageString:
		return StorageTypeNop, nil
	case jetStreamStorageString:
		return StorageTypeJetStream, nil
	default:
		return StorageTypeUndefined, eris.Errorf("invalid shard mode: %s", s)
	}
}
