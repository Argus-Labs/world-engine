package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Absorbed is the component 2c consumes. Not every entity has one, which is what the migration's
// optional input is for.
type Absorbed struct {
	B uint32
}

func (Absorbed) Name() string { return "absorbed" }

func (c Absorbed) SizeWire() int { return wire.SizeVarintField(1, c.B) }

func (c Absorbed) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.B) }

func (c Absorbed) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Absorbed{B: fields[1]}, nil
}
