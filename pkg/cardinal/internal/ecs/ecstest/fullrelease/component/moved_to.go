package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

const movedToName = "moved_to"

// MovedTo is the other half of case 2d: it has gained B. Both components survive the migration,
// which is what separates 2d from a merge, and both are produced by one migration reading both
// retired shapes.
type MovedTo struct {
	C uint32
	B uint32
}

func (MovedTo) Name() string { return movedToName }

func (c MovedTo) SizeWire() int {
	return wire.SizeVarintField(1, c.C) + wire.SizeVarintField(2, c.B)
}

func (c MovedTo) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.C)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c MovedTo) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return MovedTo{C: fields[1], B: fields[2]}, nil
}

// MovedToV1 is its retired shape, before B arrived.
type MovedToV1 struct {
	C uint32
}

func (MovedToV1) Name() string { return movedToName }

func (c MovedToV1) SizeWire() int { return wire.SizeVarintField(1, c.C) }

func (c MovedToV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.C) }

func (c MovedToV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return MovedToV1{C: fields[1]}, nil
}
