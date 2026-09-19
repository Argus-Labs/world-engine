package snapshot

import (
	"context"

	"github.com/rotisserie/eris"
)

// NopStorage is a no-op implementation of SnapshotStorage.
// It's used when snapshots are not needed (e.g., ephemeral shards, development, testing).
type NopStorage struct{}

var _ Storage = (*NopStorage)(nil)

func NewNopStorage() *NopStorage {
	return &NopStorage{}
}

func (n *NopStorage) Store(_ context.Context, _ uint64, _ []byte) error {
	return nil
}

func (n *NopStorage) Load(_ context.Context) ([]byte, error) {
	return nil, eris.Wrap(ErrSnapshotNotFound, "no snapshots available (using no-op storage)")
}
