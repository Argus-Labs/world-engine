package cardinal

import "github.com/argus-labs/cardinal/internal/entity"

// Entity is identifier of an entity.
// Entity is just a wrapper of uint64.
type Entity = entity.Entity

// Null represents an invalid entity which is zero.
var Null = entity.Null
