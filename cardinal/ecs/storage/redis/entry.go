package redis

import (
	"context"

	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

// ---------------------------------------------------------------------------
// 							ENTRY STORAGE
// ---------------------------------------------------------------------------

var _ storage.EntryStorage = &Storage{}

func (r *Storage) SetEntry(entry *types.Entry) error {
	ctx := context.Background()
	key := r.entryStorageKey(entity.ID(entry.ID))
	bz, err := marshalProto(entry)
	if err != nil {
		return err
	}
	res := r.Client.Set(ctx, key, bz, 0)
	if err := res.Err(); err != nil {
		return err
	}
	return nil
}

func (r *Storage) GetEntry(id entity.ID) (*types.Entry, error) {
	ctx := context.Background()
	key := r.entryStorageKey(id)
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err != nil {
		return nil, err
	}
	bz, err := res.Bytes()
	if err != nil {
		return nil, err
	}
	ent := new(types.Entry)
	err = unmarshalProto(bz, ent)
	if err != nil {
		return nil, err
	}
	return ent, nil
}

func (r *Storage) SetEntity(id entity.ID, e storage.Entity) error {
	entry, err := r.GetEntry(id)
	if err != nil {
		return err
	}
	entry.ID = uint64(e.ID())
	err = r.SetEntry(entry)
	if err != nil {
		return err
	}

	return nil
}

func (r *Storage) SetLocation(id entity.ID, location *types.Location) error {
	entry, err := r.GetEntry(id)
	if err != nil {
		return err
	}
	entry.Location = location
	err = r.SetEntry(entry)
	if err != nil {
		return err
	}

	return nil
}
