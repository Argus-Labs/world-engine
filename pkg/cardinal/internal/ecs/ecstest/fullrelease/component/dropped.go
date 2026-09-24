package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// DroppedV1 is case 2e: the retired shape of a component this build no longer declares.
//
// There is deliberately no current Dropped here. This file holds a retired shape and nothing else,
// which is what a dropped component looks like once the migration for it exists: the struct stays
// only long enough to read old saves, and goes when support for them does.
type DroppedV1 struct {
	A uint32
}

func (DroppedV1) Name() string { return "dropped" }

func (c DroppedV1) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c DroppedV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c DroppedV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return DroppedV1{A: fields[1]}, nil
}
