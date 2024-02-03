package query

import (
	"github.com/argus-labs/starter-game-template/cardinal/world"
	"pkg.world.dev/world-engine/cardinal"
)

type WorldVarsRequest struct {
	Key world.VarsKey `json:"key"`
}

type WorldVarsResponse struct {
	Result map[world.VarsKey]interface{}
}

func WorldVars(_ cardinal.WorldContext, req *WorldVarsRequest) (*WorldVarsResponse, error) {
	// Handle all world vars query
	if req.Key == "*" {
		return &WorldVarsResponse{Result: world.WorldVars}, nil
	}

	if value, ok := world.WorldVars[req.Key]; ok {
		mapKey := make(map[world.VarsKey]interface{})
		mapKey[req.Key] = value
		return &WorldVarsResponse{Result: mapKey}, nil
	}

	// Default case for unhandled keys
	return &WorldVarsResponse{Result: map[world.VarsKey]interface{}{}}, nil
}
