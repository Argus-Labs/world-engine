package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Split is case 2a as the current build declares it: A stays, B has moved to [SplitOff].
type Split struct {
	A uint32
}

func (Split) Name() string { return "split" }

func (c Split) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c Split) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c Split) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Split{A: fields[1]}, nil
}

// SplitV1 is the retired shape, holding both fields.
type SplitV1 struct {
	A uint32
	B uint32
}

func (SplitV1) Name() string { return "split" }

func (c SplitV1) SizeWire() int {
	return wire.SizeVarintField(1, c.A) + wire.SizeVarintField(2, c.B)
}

func (c SplitV1) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.A)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c SplitV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return SplitV1{A: fields[1], B: fields[2]}, nil
}
