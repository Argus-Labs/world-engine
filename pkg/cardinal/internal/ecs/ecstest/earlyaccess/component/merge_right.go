package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// MergeRight is the other half of case 2b. It is a component in its own right here, and an entity
// may hold one without the other, which is what makes the merge a decision rather than a rename.
type MergeRight struct {
	B uint32
}

func (MergeRight) Name() string { return "merge_right" }

func (c MergeRight) SizeWire() int { return wire.SizeVarintField(1, c.B) }

func (c MergeRight) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.B) }

func (c MergeRight) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return MergeRight{B: fields[1]}, nil
}
