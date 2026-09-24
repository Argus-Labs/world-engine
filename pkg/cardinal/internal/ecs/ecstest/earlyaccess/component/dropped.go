package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Dropped carries case 2e, drop a component. The full-release build does not declare it at all.
//
// This is the one case where the stored name has no counterpart in the current build. Every other
// case compares a stored shape against a current one; here there is nothing to compare against, so
// the value can only leave through a migration.
type Dropped struct {
	A uint32
}

func (Dropped) Name() string { return "dropped" }

func (c Dropped) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c Dropped) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c Dropped) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Dropped{A: fields[1]}, nil
}
