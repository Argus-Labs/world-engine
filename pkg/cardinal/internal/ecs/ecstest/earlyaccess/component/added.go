package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Added carries case 1a, add a field. The full-release build declares a second field that no
// stored value contains.
//
// Decoding old bytes into the new struct would leave the new field at zero. That may be right, but
// only the developer knows: zero is a real value for most types, so the engine cannot tell an
// absent field from one deliberately set to zero. A migration is how the intended value is stated.
type Added struct {
	A uint32
}

func (Added) Name() string { return "added" }

func (c Added) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c Added) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c Added) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Added{A: fields[1]}, nil
}
