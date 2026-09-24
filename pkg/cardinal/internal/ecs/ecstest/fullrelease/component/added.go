package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Added is case 1a as the current build declares it: B is new.
type Added struct {
	A uint32
	B uint32
}

func (Added) Name() string { return "added" }

func (c Added) SizeWire() int {
	return wire.SizeVarintField(1, c.A) + wire.SizeVarintField(2, c.B)
}

func (c Added) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.A)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c Added) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Added{A: fields[1], B: fields[2]}, nil
}

// AddedV1 is the retired shape, without B.
type AddedV1 struct {
	A uint32
}

func (AddedV1) Name() string { return "added" }

func (c AddedV1) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c AddedV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c AddedV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return AddedV1{A: fields[1]}, nil
}
