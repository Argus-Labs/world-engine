package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Spawner carries case 3b, create entities during a migration. Count is how many entities the
// full-release build has to bring into existence for this one.
type Spawner struct {
	Count uint32
}

func (Spawner) Name() string { return "spawner" }

func (c Spawner) SizeWire() int { return wire.SizeVarintField(1, c.Count) }

func (c Spawner) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.Count) }

func (c Spawner) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Spawner{Count: fields[1]}, nil
}
