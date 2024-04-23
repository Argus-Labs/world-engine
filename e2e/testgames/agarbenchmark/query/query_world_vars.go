package query

import (
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/world"

	"pkg.world.dev/world-engine/cardinal"
)

type WorldVarsRequest struct{}

type WorldVarsResponse struct {
	Result world.GameSettingsJSON
}

func WorldVars(_ cardinal.WorldContext, _ *WorldVarsRequest) (*WorldVarsResponse, error) {
	return &WorldVarsResponse{Result: world.Settings.GameSettingsJSONAble()}, nil
}
