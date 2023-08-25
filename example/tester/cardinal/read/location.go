package read

import (
	"fmt"
	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/sys"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type LocationRequest struct {
	ID string
}

type LocationReply struct {
	X, Y int64
}

var Location = ecs.NewReadType[LocationRequest, LocationReply]("location", func(world *ecs.World, req LocationRequest) (LocationReply, error) {
	playerEntityID, ok := sys.PlayerEntityID[req.ID]
	if !ok {
		return LocationReply{}, fmt.Errorf("player does not exist")
	}
	loc, err := comp.LocationComponent.Get(world, playerEntityID)
	if err != nil {
		return LocationReply{}, err
	}
	return LocationReply{
		X: loc.Y,
		Y: loc.X,
	}, nil
}, ecs.WithReadEVMSupport[LocationRequest, LocationReply])
