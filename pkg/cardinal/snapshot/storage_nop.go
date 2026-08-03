package snapshot

import (
	"context"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
)

// NopStorage is a no-op implementation of SnapshotStorage.
// It's used when snapshots are not needed (e.g., development, testing).
// Store does no work at all: serialization happens inside the backends, so nothing is
// marshaled on behalf of a snapshot that is then discarded.
type NopStorage struct{}

var _ Storage = (*NopStorage)(nil)

// NewNopStorage creates a new no-op snapshot storage.
func NewNopStorage() *NopStorage {
	return &NopStorage{}
}

func (n *NopStorage) Store(_ context.Context, _ *cardinalv1.Snapshot) error {
	return nil
}

func (n *NopStorage) Load(_ context.Context) (*cardinalv1.Snapshot, error) {
	return nil, eris.Wrap(ErrSnapshotNotFound, "no snapshots available (using no-op storage)")
}
