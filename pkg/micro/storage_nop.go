package micro

import "github.com/rotisserie/eris"

// NopSnapshotStorage is a no-op implementation of SnapshotStorage.
// It's used when snapshots are not needed (e.g., development, testing).
type NopSnapshotStorage struct{}

var _ SnapshotStorage = (*NopSnapshotStorage)(nil)

// NewNopSnapshotStorage creates a new no-op snapshot storage.
func NewNopSnapshotStorage() *NopSnapshotStorage {
	return &NopSnapshotStorage{}
}

func (n *NopSnapshotStorage) Store(_ *Snapshot) error {
	return nil
}

func (n *NopSnapshotStorage) Load() (*Snapshot, error) {
	return nil, eris.New("no snapshots available (using no-op storage)")
}

func (n *NopSnapshotStorage) Exists() bool {
	return false
}
