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

// CurrentVersion is the snapshot format this build writes. Bump it whenever the envelope or the
// world state inside it stops being readable by the previous layout, and teach ValidateVersion and
// the restore path how to read the older versions that are still accepted.
const CurrentVersion uint32 = 1

var (
	ErrSnapshotNotFound = errors.New("snapshot not found")

	// ErrUnsupportedVersion reports a snapshot this build must not interpret.
	ErrUnsupportedVersion = errors.New("unsupported snapshot version")
)

// ValidateVersion reports whether a stored snapshot may be interpreted by this build.
//
// The policy, which the format has to keep to:
//   - 1 <= version <= CurrentVersion is accepted. Older formats stay readable, so a future bump
//     must leave the restore path able to read every earlier version. There is only version 1
//     today, so nothing is asked of the reader yet.
//   - version > CurrentVersion is rejected. The snapshot comes from a newer build whose layout this
//     one does not know. Protobuf decodes unknown layouts happily, so without this check a newer
//     snapshot would be silently mis-read into a wrong world rather than refused.
//   - version == 0 is rejected. The field is unset, meaning either a writer from before the
//     envelope carried a version or a corrupt envelope. Every writer since the field existed sets
//     it, so 0 is never something this code could have produced.
func ValidateVersion(version uint32) error {
	switch {
	case version == 0:
		return eris.Wrapf(ErrUnsupportedVersion,
			"snapshot carries no version, so it predates the versioned format and cannot be read as version %d",
			CurrentVersion)
	case version > CurrentVersion:
		return eris.Wrapf(ErrUnsupportedVersion,
			"snapshot is version %d but this build only reads up to version %d", version, CurrentVersion)
	default:
		return nil
	}
}

// Storage provides persistence for shard snapshots.
//
// Implementations take the snapshot envelope as a protobuf message and serialize it themselves,
// so the envelope is marshaled exactly once on the write path and unmarshaled exactly once on
// the read path.
type Storage interface {
	// Store saves the snapshot, atomically replacing any existing snapshot.
	// No implementation retains the replaced snapshot: JetStream purges the superseded chunks
	// as soon as the new object commits, and S3 leaves retention to the bucket's own versioning
	// configuration.
	Store(ctx context.Context, snapshot *cardinalv1.Snapshot) error

	// Load retrieves the current snapshot.
	// Returns an error wrapping ErrSnapshotNotFound if no snapshot exists.
	Load(ctx context.Context) (*cardinalv1.Snapshot, error)
}

// marshalSnapshot encodes the snapshot envelope for a backend to hand to its transport.
// Backends must call this exactly once per Store so that the on-disk bytes stay identical
// across implementations.
func marshalSnapshot(snapshot *cardinalv1.Snapshot) ([]byte, error) {
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
