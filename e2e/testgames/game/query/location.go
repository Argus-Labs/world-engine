package query

import (
	"errors"

	"github.com/argus-labs/world-engine/example/tester/game/comp"
	"github.com/argus-labs/world-engine/example/tester/game/sys"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/query"
)

type LocationRequest struct {
	ID string
}

type LocationReply struct {
	X, Y int64
}

func RegisterLocationQuery(world *cardinal.World) error {
	return cardinal.RegisterQuery[LocationRequest, LocationReply](
		world,
		"location",
		func(ctx cardinal.WorldContext, req *LocationRequest) (*LocationReply, error) {
			playerEntityID, ok := sys.PlayerEntityID[req.ID]
			if !ok {
				ctx.Logger().Info().Msg("listing existing players...")
				for playerID := range sys.PlayerEntityID {
					ctx.Logger().Info().Msg(playerID)
				}
				return &LocationReply{}, errors.New("player does not exist")
			}
			loc, err := cardinal.GetComponent[comp.Location](ctx, playerEntityID)
			if err != nil {
				return &LocationReply{}, err
			}
			return &LocationReply{
				X: loc.X,
				Y: loc.Y,
			}, nil
		},
		query.WithQueryEVMSupport[LocationRequest, LocationReply]())
}
