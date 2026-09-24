package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// MergeLeft is half of case 2b, merge many into a new component. The full-release build declares
// neither this nor [MergeRight]; both are replaced by a single new component.
type MergeLeft struct {
	A uint32
}

func (MergeLeft) Name() string { return "merge_left" }

func (c MergeLeft) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c MergeLeft) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c MergeLeft) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return MergeLeft{A: fields[1]}, nil
}
