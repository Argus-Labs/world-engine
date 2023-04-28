package storage

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"

	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

func NewEntry(id entity.ID, loc *types.Location) *types.Entry {
	return &types.Entry{
		ID:       uint64(id),
		Location: loc,
	}
}
