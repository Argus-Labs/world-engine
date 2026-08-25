package snapshot

import (
	"context"
	"errors"
	"strings"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/assert"
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

// Storage persists shard snapshots.
//
// Implementations serialize Snapshot messages. Store and Load define ownership of their messages.
type Storage interface {
	// Store saves the snapshot, atomically replacing any existing snapshot.
	//
	// Ownership: The caller keeps the message. Store must treat it as read-only.
	//
	// Before Store returns, it must marshal the message or make a deep copy. It must not retain or
	// use the caller's message, or its data, after it returns.
	//
	// Cardinal calls Store synchronously, with the newest snapshot first. Store must not start a
	// background write that can change this order.
	Store(ctx context.Context, snapshot *cardinalv1.Snapshot) error

	// Load retrieves the current snapshot, returns an error if no snapshot exists.
	//
	// Ownership: The returned message belongs only to the caller. Load must return a newly decoded
	// message or a deep copy. It must not read or write the message after it returns.
	Load(ctx context.Context) (*cardinalv1.Snapshot, error)
}

// marshalSnapshot encodes a snapshot into independent bytes.
func marshalSnapshot(snapshot *cardinalv1.Snapshot) ([]byte, error) {
	assert.That(snapshot.GetVersion() == CurrentVersion,
		"snapshot version must be %d, got %d", CurrentVersion, snapshot.GetVersion())

	// Deterministic only sorts map entries, and neither Snapshot nor WorldState declares a map
	// field today, so it is currently a no-op that costs nothing. It is kept so the intent
	// survives if a map field is ever added to the envelope.
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal snapshot")
	}
	return data, nil
}

// unmarshalSnapshot decodes and validates bytes previously written by marshalSnapshot.
func unmarshalSnapshot(data []byte) (*cardinalv1.Snapshot, error) {
	snapshot := &cardinalv1.Snapshot{}
	if err := proto.Unmarshal(data, snapshot); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal snapshot")
	}
	if err := protovalidate.Validate(snapshot); err != nil {
		return nil, eris.Wrap(err, "failed to validate snapshot")
	}
	// Checked here, at the decode boundary, so bytes this build cannot interpret never become a
	// world state graph. Callers holding a Snapshot from elsewhere must call ValidateVersion too.
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
