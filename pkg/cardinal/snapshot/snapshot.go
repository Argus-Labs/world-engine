package snapshot

import (
	"context"
	"errors"
	"strings"

	"buf.build/go/protovalidate"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

// CurrentVersion is the snapshot format this build writes and the only one it reads. Bump it
// whenever the envelope or the world state inside it stops being readable by the previous layout,
// and see ValidateVersion for what a bump obliges you to write first.
const CurrentVersion uint32 = 1

var (
	ErrSnapshotNotFound = errors.New("snapshot not found")

	// ErrUnsupportedVersion reports a snapshot this build must not interpret.
	ErrUnsupportedVersion = errors.New("unsupported snapshot version")
)

// ValidateVersion reports whether a snapshot uses the format this build can read. Only
// CurrentVersion is valid; all other versions are rejected.
func ValidateVersion(version uint32) error {
	if version == CurrentVersion {
		return nil
	}
	return eris.Wrapf(ErrUnsupportedVersion, "expected version %d, got %d", CurrentVersion, version)
}

// Storage persists shard snapshots as opaque bytes.
//
// The bytes are a fully encoded Snapshot envelope, produced by the engine's streaming encoder.
// Storage never decodes them: writing and reading are pure byte transport, so the encoder's
// zero-allocation work isn't undone at the boundary.
type Storage interface {
	// Store saves the snapshot bytes, atomically replacing any existing snapshot. The tick is
	// metadata for logging and keys; the bytes are the record.
	//
	// Ownership: data is handed off. The caller never touches it again, so Store may retain it,
	// but must not modify it.
	//
	// Cardinal calls Store synchronously, with the newest snapshot first. Store must not start a
	// background write that can change this order.
	Store(ctx context.Context, tick uint64, data []byte) error

	// Load retrieves the current snapshot bytes, or an error wrapping ErrSnapshotNotFound if no
	// snapshot exists. The caller owns the returned slice.
	Load(ctx context.Context) ([]byte, error)
}

// Decode parses and validates snapshot bytes into the envelope message.
//
// This is the one decode boundary: bytes from storage become a world-state graph here or not at
// all, so version refusal happens before any interpretation. A boot path — allocations are fine.
func Decode(data []byte) (*cardinalv1.Snapshot, error) {
	snapshot := &cardinalv1.Snapshot{}
	if err := proto.Unmarshal(data, snapshot); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal snapshot")
	}
	if err := protovalidate.Validate(snapshot); err != nil {
		return nil, eris.Wrap(err, "failed to validate snapshot")
	}
	if err := ValidateVersion(snapshot.GetVersion()); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// StorageType defines the type of snapshot storage to use.
type StorageType uint8

const (
	StorageTypeUndefined StorageType = iota
	StorageTypeNop
	StorageTypeJetStream
	StorageTypeS3
)

const (
	nopStorageString       = "NOP"
	jetStreamStorageString = "JETSTREAM"
	s3StorageString        = "S3"
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
	case StorageTypeS3:
		return s3StorageString
	default:
		return undefinedStorageString
	}
}

func (s StorageType) IsValid() bool {
	return s == StorageTypeNop || s == StorageTypeJetStream || s == StorageTypeS3
}

func ParseStorageType(s string) (StorageType, error) {
	switch strings.ToUpper(s) {
	case nopStorageString:
		return StorageTypeNop, nil
	case jetStreamStorageString:
		return StorageTypeJetStream, nil
	case s3StorageString:
		return StorageTypeS3, nil
	default:
		return StorageTypeUndefined, eris.Errorf("invalid shard mode: %s", s)
	}
}
