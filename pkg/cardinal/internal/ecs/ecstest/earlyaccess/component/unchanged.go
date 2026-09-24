package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Unchanged is the control. The full-release build declares it identically, so its stored shape
// still matches the current code and no migration may run for it.
//
// Without a control, a runtime that converted every component it found would pass every test in
// this fixture.
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
