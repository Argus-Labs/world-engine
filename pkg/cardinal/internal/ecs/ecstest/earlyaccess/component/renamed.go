package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Renamed carries case 1c, rename a field. The full-release build declares the same component with
// this field called After.
//
// A rename moves no bytes: the field keeps its number and its wire type, so a stored value decodes
// into the new struct without complaint and with the right number in it. That is precisely why it
// needs a migration declared rather than being left alone — the engine cannot tell a rename from a
// coincidence, and only the developer knows the field still means the same thing.
type Renamed struct {
	Before uint32
}

func (Renamed) Name() string { return "renamed" }

func (c Renamed) SizeWire() int { return wire.SizeVarintField(1, c.Before) }

func (c Renamed) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.Before) }

func (c Renamed) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Renamed{Before: fields[1]}, nil
}
