package snapshot

import (
	"context"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
)

// NopStorage is a no-op implementation of SnapshotStorage.
// It's used when snapshots are not needed (e.g., ephemeral shards, development, testing).
type NopStorage struct{}

var _ Storage = (*NopStorage)(nil)

func NewNopStorage() *NopStorage {
	return &NopStorage{}
}

func (n *NopStorage) Store(_ context.Context, _ *cardinalv1.Snapshot) error {
	return nil
}

func (n *NopStorage) Load(_ context.Context) (*cardinalv1.Snapshot, error) {
	return nil, eris.Wrap(ErrSnapshotNotFound, "no snapshots available (using no-op storage)")
}
