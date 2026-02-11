package snapshot

import (
	"context"

	"github.com/rotisserie/eris"
)

// NopStorage is a no-op implementation of SnapshotStorage.
// It's used when snapshots are not needed (e.g., development, testing).
type NopStorage struct{}

var _ Storage = (*NopStorage)(nil)

// NewNopStorage creates a new no-op snapshot storage.
func NewNopStorage() *NopStorage {
	return &NopStorage{}
}

func (n *NopStorage) Store(_ context.Context, _ *Snapshot) error {
	return nil
}

func (n *NopStorage) Load(_ context.Context) (*Snapshot, error) {
	return nil, eris.Wrap(ErrSnapshotNotFound, "no snapshots available (using no-op storage)")
}
