// Package component holds the early-access component shapes: the shapes a stored snapshot was
// written from. It is a test fixture and not part of the engine.
//
// These shapes are frozen. Changing one changes what "the old save" means, which is the one thing
// the fixture exists to pin down. New migration cases arrive as new components, never by editing an
// existing one.
//
// See the README two directories up.
package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Chained carries the chain case: a component that changed twice, with a save written before either
// change.
//
// This is the early-access shape, the one that reached disk. The full-release build never sees a
// save in the shape that came after it, because that build shipped and nobody saved from it — but
// the migration for it was still written, and still has to run.
type Chained struct {
	A uint32
}

func (Chained) Name() string { return "chained" }

func (c Chained) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c Chained) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c Chained) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Chained{A: fields[1]}, nil
}
