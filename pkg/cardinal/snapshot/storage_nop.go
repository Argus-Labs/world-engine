package snapshot

import "github.com/rotisserie/eris"

// NopStorage is a no-op implementation of SnapshotStorage.
// It's used when snapshots are not needed (e.g., development, testing).
type NopStorage struct{}

var _ Storage = (*NopStorage)(nil)

// NewNopStorage creates a new no-op snapshot storage.
func NewNopStorage() *NopStorage {
	return &NopStorage{}
}

func (n *NopStorage) Store(_ *Snapshot) error {
	return nil
}

func (n *NopStorage) Load() (*Snapshot, error) {
	return nil, eris.New("no snapshots available (using no-op storage)")
}

func (n *NopStorage) Exists() bool {
	return false
}
