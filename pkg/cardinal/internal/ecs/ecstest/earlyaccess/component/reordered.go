package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Reordered carries case 1d, reorder fields. The full-release build declares the same two fields
// the other way round.
//
// This is the case with the worst failure if it goes unnoticed. The generator numbers fields in
// declaration order, so swapping them swaps their numbers: A is field 1 here and field 2 there.
// Old bytes decoded by the new struct put A's value into B and B's into A, silently, with no
// error and no type mismatch. The shape hash covers declaration order for exactly this reason.
type Reordered struct {
	A uint32
	B uint32
}

func (Reordered) Name() string { return "reordered" }

func (c Reordered) SizeWire() int {
	return wire.SizeVarintField(1, c.A) + wire.SizeVarintField(2, c.B)
}

func (c Reordered) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.A)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c Reordered) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Reordered{A: fields[1], B: fields[2]}, nil
}
