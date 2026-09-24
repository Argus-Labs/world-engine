package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Removed is case 1b as the current build declares it: B is gone.
type Removed struct {
	A uint32
}

func (Removed) Name() string { return "removed" }

func (c Removed) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c Removed) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c Removed) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Removed{A: fields[1]}, nil
}

// RemovedV1 is the retired shape, still carrying B. Keeping it is what gives the migration access
// to a value the current struct has no room for.
type RemovedV1 struct {
	A uint32
	B uint32
}

func (RemovedV1) Name() string { return "removed" }

func (c RemovedV1) SizeWire() int {
	return wire.SizeVarintField(1, c.A) + wire.SizeVarintField(2, c.B)
}

func (c RemovedV1) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.A)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c RemovedV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return RemovedV1{A: fields[1], B: fields[2]}, nil
}
