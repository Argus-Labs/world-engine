package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

const movedFromName = "moved_from"

// MovedFrom is case 2d as the current build declares it: B has gone to [MovedTo].
type MovedFrom struct {
	A uint32
}

func (MovedFrom) Name() string { return movedFromName }

func (c MovedFrom) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c MovedFrom) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c MovedFrom) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return MovedFrom{A: fields[1]}, nil
}

// MovedFromV1 is its retired shape, still holding B.
type MovedFromV1 struct {
	A uint32
	B uint32
}

func (MovedFromV1) Name() string { return movedFromName }

func (c MovedFromV1) SizeWire() int {
	return wire.SizeVarintField(1, c.A) + wire.SizeVarintField(2, c.B)
}

func (c MovedFromV1) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.A)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c MovedFromV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return MovedFromV1{A: fields[1], B: fields[2]}, nil
}
