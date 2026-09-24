package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Minion is what case 3b creates. No save holds one and no migration converts anything into one:
// entities carrying it come into existence during the restore, which is the only way this
// component ever appears.
//
// Owner is the entity that spawned it, which is also the simplest form of the reference case 3c
// deals with: it is correct here only because the spawner already existed when the minion was made.
type Minion struct {
	Owner uint32
}

func (Minion) Name() string { return "minion" }

func (c Minion) SizeWire() int { return wire.SizeVarintField(1, c.Owner) }

func (c Minion) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.Owner) }

func (c Minion) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Minion{Owner: fields[1]}, nil
}
