package storage

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
)

// Entity is identifier of an Ent.
// Entity is just a wrapper of uint64.
type Entity = entity.Entity

// Null represents an invalid Ent which is zero.
var Null = entity.Null
