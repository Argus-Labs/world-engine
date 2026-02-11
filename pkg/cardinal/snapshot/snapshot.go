package snapshot

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/rotisserie/eris"
)

// Snapshot represents a point-in-time capture of shard state.
// This is an alias to the protobuf-generated type for better API ergonomics.
type Snapshot struct {
	TickHeight uint64
	Timestamp  time.Time
	Data       []byte
	Version    uint32
}

const CurrentVersion uint32 = 1

var ErrSnapshotNotFound = errors.New("snapshot not found")

// Storage provides persistence for shard snapshots.
// Implementations handle atomic storage with automatic backup of previous snapshots.
type Storage interface {
	// Store saves the snapshot, atomically replacing any existing snapshot.
	// The previous snapshot should be preserved as backup if possible.
	Store(ctx context.Context, snapshot *Snapshot) error

	// Load retrieves the current snapshot.
	// Returns an error if no snapshot exists.
	Load(ctx context.Context) (*Snapshot, error)
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

func (s StorageType) IsValid() bool {
	return s == StorageTypeNop || s == StorageTypeJetStream
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
