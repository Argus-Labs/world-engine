// Package component holds a plugin's current component shapes, with the retired shapes its earlier
// releases wrote. It is a test fixture and not part of the engine.
//
// The plugin has shipped three times and the field has been replaced twice, so a save from the
// first release is two conversions away from what this build declares. Nothing about that is
// specific to plugins — a shard can put its own component through as many versions — but a plugin
// is where it actually happens, because a plugin author has no say in how old their users' saves
// are.
package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// providedName is shared by the three shapes below. A snapshot identifies a component by name, so
// a retired shape only receives the old bytes if its name matches the current one exactly.
const providedName = "provided"

// Provided is the shape this release declares.
type Provided struct {
	C uint32
}

func (Provided) Name() string { return providedName }

func (c Provided) SizeWire() int { return wire.SizeVarintField(1, c.C) }

func (c Provided) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.C) }

func (c Provided) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Provided{C: fields[1]}, nil
}

// ProvidedV1 is the retired shape the plugin's first release wrote, and the one the save in the
// test holds.
type ProvidedV1 struct {
	A uint32
}

func (ProvidedV1) Name() string { return providedName }

func (c ProvidedV1) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c ProvidedV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c ProvidedV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return ProvidedV1{A: fields[1]}, nil
}

// ProvidedV2 is the retired shape the second release wrote. A save can hold one directly, and a
// save from the first release passes through it on the way here.
type ProvidedV2 struct {
	B uint32
}

func (ProvidedV2) Name() string { return providedName }

func (c ProvidedV2) SizeWire() int { return wire.SizeVarintField(1, c.B) }

func (c ProvidedV2) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.B) }

func (c ProvidedV2) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return ProvidedV2{B: fields[1]}, nil
}
