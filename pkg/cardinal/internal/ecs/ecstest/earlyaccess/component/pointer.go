package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Pointer carries case 3c, fix references between entities. It names its target by a slot number,
// which the full-release build replaces with the target entity's own ID.
type Pointer struct {
	Slot uint32
}

func (Pointer) Name() string { return "pointer" }

func (c Pointer) SizeWire() int { return wire.SizeVarintField(1, c.Slot) }

func (c Pointer) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.Slot) }

func (c Pointer) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Pointer{Slot: fields[1]}, nil
}
