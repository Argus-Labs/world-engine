package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Reordered is case 1d as the current build declares it: B is now first, so B is field 1 and A is
// field 2. The stored bytes have them the other way round.
type Reordered struct {
	B uint32
	A uint32
}

func (Reordered) Name() string { return "reordered" }

func (c Reordered) SizeWire() int {
	return wire.SizeVarintField(1, c.B) + wire.SizeVarintField(2, c.A)
}

func (c Reordered) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.B)
	return wire.AppendVarintField(b, 2, c.A)
}

func (c Reordered) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Reordered{B: fields[1], A: fields[2]}, nil
}

// ReorderedV1 is the retired shape, with A first. It reads field 1 as A, which is what the old
// build wrote there.
type ReorderedV1 struct {
	A uint32
	B uint32
}

func (ReorderedV1) Name() string { return "reordered" }

func (c ReorderedV1) SizeWire() int {
	return wire.SizeVarintField(1, c.A) + wire.SizeVarintField(2, c.B)
}

func (c ReorderedV1) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.A)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c ReorderedV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return ReorderedV1{A: fields[1], B: fields[2]}, nil
}
