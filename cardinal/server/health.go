package server

import (
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware/untyped"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

func (handler *Handler) registerHealthHandlerSwagger(world *ecs.World, api *untyped.API) error {
	healthHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		res := struct {
			IsServerRunning   bool `json:"is_server_running"`
			IsGameLoopRunning bool `json:"is_game_loop_running"`
		}{
			true, //see http://ismycomputeron.com/
			world.IsGameLoopRunning()}
		return res, nil
	})
	api.RegisterOperation("GET", "/health", healthHandler)
	return nil
}
