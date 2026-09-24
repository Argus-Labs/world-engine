package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Slotted is what [Pointer] refers to. It is unchanged between the two builds, so it is the
// reference that migrates, not the thing referred to.
type Slotted struct {
	Number uint32
}

func (Slotted) Name() string { return "slotted" }

func (c Slotted) SizeWire() int { return wire.SizeVarintField(1, c.Number) }

func (c Slotted) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.Number) }

func (c Slotted) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Slotted{Number: fields[1]}, nil
}
