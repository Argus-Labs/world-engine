package query

import (
	"fmt"
	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/sys"
	"pkg.world.dev/world-engine/cardinal"
)

type LocationRequest struct {
	ID string
}

type LocationReply struct {
	X, Y int64
}

var Location = cardinal.NewQueryTypeWithEVMSupport[LocationRequest, LocationReply]("location", func(ctx cardinal.WorldContext, req LocationRequest) (LocationReply, error) {
	playerEntityID, ok := sys.PlayerEntityID[req.ID]
	if !ok {
		return LocationReply{}, fmt.Errorf("player does not exist")
	}
	loc, err := cardinal.GetComponent[comp.Location](ctx, playerEntityID)
	if err != nil {
		return LocationReply{}, err
	}
	return LocationReply{
		X: loc.Y,
		Y: loc.X,
	}, nil
})
