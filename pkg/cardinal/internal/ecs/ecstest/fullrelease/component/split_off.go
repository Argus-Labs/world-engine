package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// SplitOff is the component B moved into. No save holds one: it exists only because a migration
// produces it, which is what makes the entity change archetype during restore.
type SplitOff struct {
	B uint32
}

func (SplitOff) Name() string { return "split_off" }

func (c SplitOff) SizeWire() int { return wire.SizeVarintField(1, c.B) }

func (c SplitOff) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.B) }

func (c SplitOff) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return SplitOff{B: fields[1]}, nil
}
