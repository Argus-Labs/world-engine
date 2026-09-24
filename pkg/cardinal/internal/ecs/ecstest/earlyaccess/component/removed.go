package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Removed carries case 1b, remove a field. The full-release build drops B.
//
// The stored bytes still contain field 2, and a decoder that ignored it would be right here — but
// only because nothing needed the value. A migration is what lets the developer do something with
// it first, and the same declaration covers both.
type Removed struct {
	A uint32
	B uint32
}

func (Removed) Name() string { return "removed" }

func (c Removed) SizeWire() int {
	return wire.SizeVarintField(1, c.A) + wire.SizeVarintField(2, c.B)
}

func (c Removed) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.A)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c Removed) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Removed{A: fields[1], B: fields[2]}, nil
}
