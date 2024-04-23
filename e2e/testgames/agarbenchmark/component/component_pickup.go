package component

import "github.com/argus-labs/world-engine/example/tester/agarbenchmark/world"

type Pickup struct {
	ID world.PickupType
}

func (Pickup) Name() string {
	return "Pickup"
}
