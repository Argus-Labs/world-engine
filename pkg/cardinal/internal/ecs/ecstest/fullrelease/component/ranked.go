package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

const rankedName = "ranked"

// Ranked is case 3a as the current build declares it: a score, and a position among every other
// entity that has one.
//
// Rank is why this case needs a whole-world migration. The per-entity migration below can carry
// the score across and nothing else, because the answer for one entity depends on all of them.
type Ranked struct {
	Score uint32
	Rank  uint32
}

func (Ranked) Name() string { return rankedName }

func (c Ranked) SizeWire() int {
	return wire.SizeVarintField(1, c.Score) + wire.SizeVarintField(2, c.Rank)
}

func (c Ranked) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.Score)
	return wire.AppendVarintField(b, 2, c.Rank)
}

func (c Ranked) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Ranked{Score: fields[1], Rank: fields[2]}, nil
}

// RankedV1 is the retired shape, before rank existed.
type RankedV1 struct {
	Score uint32
}

func (RankedV1) Name() string { return rankedName }

func (c RankedV1) SizeWire() int { return wire.SizeVarintField(1, c.Score) }

func (c RankedV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.Score) }

func (c RankedV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return RankedV1{Score: fields[1]}, nil
}
