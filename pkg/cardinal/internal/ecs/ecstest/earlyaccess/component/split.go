package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Split carries case 2a, split one component into many. The full-release build keeps A here and
// moves B into a component of its own.
//
// The entity ends up holding a component its save never contained, so it changes archetype. That
// is why the engine works out an entity's component set from the migration plan before decoding
// any payload, rather than from what the snapshot lists.
type Split struct {
	A uint32
	B uint32
}

func (Split) Name() string { return "split" }

func (c Split) SizeWire() int {
	return wire.SizeVarintField(1, c.A) + wire.SizeVarintField(2, c.B)
}

func (c Split) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.A)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c Split) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Split{A: fields[1], B: fields[2]}, nil
}
