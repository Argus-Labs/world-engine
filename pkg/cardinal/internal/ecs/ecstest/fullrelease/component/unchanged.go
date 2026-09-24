package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Unchanged is the control, declared exactly as the early-access build declared it. Its stored shape
// still matches, so restoring a save must carry its values over untouched and run nothing for it.
type Unchanged struct {
	A uint32
}

func (Unchanged) Name() string { return "unchanged" }

func (c Unchanged) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c Unchanged) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c Unchanged) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Unchanged{A: fields[1]}, nil
}
