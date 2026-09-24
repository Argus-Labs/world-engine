package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

const spawnerName = "spawner"

// Spawner is case 3b as the current build declares it: it records how many entities were made for
// it, which no per-entity migration could fill in, because making them is not something an entity
// can do to itself.
type Spawner struct {
	Count   uint32
	Spawned uint32
}

func (Spawner) Name() string { return spawnerName }

func (c Spawner) SizeWire() int {
	return wire.SizeVarintField(1, c.Count) + wire.SizeVarintField(2, c.Spawned)
}

func (c Spawner) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.Count)
	return wire.AppendVarintField(b, 2, c.Spawned)
}

func (c Spawner) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Spawner{Count: fields[1], Spawned: fields[2]}, nil
}

// SpawnerV1 is the retired shape, before the count of created entities was recorded.
type SpawnerV1 struct {
	Count uint32
}

func (SpawnerV1) Name() string { return spawnerName }

func (c SpawnerV1) SizeWire() int { return wire.SizeVarintField(1, c.Count) }

func (c SpawnerV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.Count) }

func (c SpawnerV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return SpawnerV1{Count: fields[1]}, nil
}
