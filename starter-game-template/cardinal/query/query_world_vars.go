package query

import (
	"github.com/argus-labs/starter-game-template/cardinal/world"
	"pkg.world.dev/world-engine/cardinal"
)

type WorldVarsRequest struct {
	Key world.WorldVarsKey `json:"key"`
}

type WorldVarsResponse struct {
	Result map[world.WorldVarsKey]interface{}
}

func WorldVars(_ cardinal.WorldContext, req *WorldVarsRequest) (*WorldVarsResponse, error) {
	// Handle all world vars query
	if req.Key == "*" {
		return &WorldVarsResponse{Result: world.WorldVars}, nil
	} else {
		if value, ok := world.WorldVars[req.Key]; ok {
			return &WorldVarsResponse{Result: map[world.WorldVarsKey]interface{}{req.Key: value}}, nil
		} else {
			return &WorldVarsResponse{Result: map[world.WorldVarsKey]interface{}{}}, nil
		}
	}
}
