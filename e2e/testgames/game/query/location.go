package query

import (
	"errors"

	"github.com/argus-labs/world-engine/example/tester/game/comp"
	"github.com/argus-labs/world-engine/example/tester/game/sys"

	"pkg.world.dev/world-engine/cardinal/world"
)

type LocationReq struct {
	ID string
}

type LocationResp struct {
	X, Y int64
}

func Location(ctx world.WorldContextReadOnly, req *LocationReq) (*LocationResp, error) {
	playerEntityID, ok := sys.PlayerEntityID[req.ID]
	if !ok {
		ctx.Logger().Info().Msg("listing existing players...")
		for playerID := range sys.PlayerEntityID {
			ctx.Logger().Info().Msg(playerID)
		}
		return &LocationResp{}, errors.New("player does not exist")
	}
	loc, err := world.GetComponent[comp.Location](ctx, playerEntityID)
	if err != nil {
		return &LocationResp{}, err
	}
	return &LocationResp{
		X: loc.X,
		Y: loc.Y,
	}, nil
}
