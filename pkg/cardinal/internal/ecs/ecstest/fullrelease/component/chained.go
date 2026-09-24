// Package component holds the full-release component shapes: the shapes the current code declares,
// alongside the retired shapes older builds wrote. It is a test fixture and not part of the engine.
//
// Component names match the early-access package exactly. A snapshot records a component by name
// and nothing else, so the name is the only thing tying a stored value to the struct that receives
// it.
//
// See the README two directories up.
package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Chained is the chain case as the current build declares it: A is gone, B arrived in the shape
// before this one, and C is new here.
//
// Reaching this from the stored shape takes two conversions, and no single migration in this
// package performs both. That is the point of the case — a developer writes one migration per
// version step, and the engine composes them.
type Chained struct {
	B uint32
	C uint32
}

// chainedName is shared by the three shapes below. A snapshot identifies a component by name, so a
// retired shape only receives the old bytes if its name matches the current one exactly.
const chainedName = "chained"

func (Chained) Name() string { return chainedName }

func (c Chained) SizeWire() int {
	return wire.SizeVarintField(1, c.B) + wire.SizeVarintField(2, c.C)
}

func (c Chained) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.B)
	return wire.AppendVarintField(b, 2, c.C)
}

func (c Chained) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Chained{B: fields[1], C: fields[2]}, nil
}

// ChainedV1 is the retired shape the early-access build wrote, and the only one of the two retired
// shapes below that a save can actually hold.
type ChainedV1 struct {
	A uint32
}

func (ChainedV1) Name() string { return chainedName }

func (c ChainedV1) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c ChainedV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c ChainedV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return ChainedV1{A: fields[1]}, nil
}

// ChainedV2 is a retired shape that no save holds. A build in between declared it, added B, and
// shipped before anyone saved from it.
//
// It is still declared, and its migration is still registered, because a developer converting a
// component twice writes one migration per step and has no way to know which steps a given save
// skipped. The engine finds the route from what a save actually holds to what the code declares;
// this shape is a waypoint on that route and never touches disk in either direction.
type ChainedV2 struct {
	A uint32
	B uint32
}

func (ChainedV2) Name() string { return chainedName }

func (c ChainedV2) SizeWire() int {
	return wire.SizeVarintField(1, c.A) + wire.SizeVarintField(2, c.B)
}

func (c ChainedV2) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.A)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c ChainedV2) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return ChainedV2{A: fields[1], B: fields[2]}, nil
}
