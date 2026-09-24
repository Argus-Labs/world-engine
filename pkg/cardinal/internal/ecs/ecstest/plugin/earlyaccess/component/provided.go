// Package component holds a plugin's component shapes as an early release declared them. It is a
// test fixture and not part of the engine.
//
// A plugin owns its components outright: the shard using it cannot edit them, and the two ship on
// their own schedules. That is the only thing separating this package from the shard fixture
// beside it, and the engine cannot tell the difference at all.
package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Provided is a component a plugin provides, in the shape its earliest release wrote.
type Provided struct {
	A uint32
}

func (Provided) Name() string { return "provided" }

func (c Provided) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c Provided) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c Provided) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Provided{A: fields[1]}, nil
}
