package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Ranked carries case 3a, compute from other entities. A rank is a position among every other
// entity, so nothing inside one entity can work it out.
type Ranked struct {
	Score uint32
}

func (Ranked) Name() string { return "ranked" }

func (c Ranked) SizeWire() int { return wire.SizeVarintField(1, c.Score) }

func (c Ranked) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.Score) }

func (c Ranked) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Ranked{Score: fields[1]}, nil
}
