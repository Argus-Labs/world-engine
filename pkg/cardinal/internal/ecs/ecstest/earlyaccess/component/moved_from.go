package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// MovedFrom is the other half of case 2d. B leaves here and reappears on [MovedTo].
type MovedFrom struct {
	A uint32
	B uint32
}

func (MovedFrom) Name() string { return "moved_from" }

func (c MovedFrom) SizeWire() int {
	return wire.SizeVarintField(1, c.A) + wire.SizeVarintField(2, c.B)
}

func (c MovedFrom) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.A)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c MovedFrom) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return MovedFrom{A: fields[1], B: fields[2]}, nil
}
