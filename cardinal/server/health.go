package server

import (
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware/untyped"
)

type HealthResponse struct {
	IsServerRunning   bool `json:"is_server_running"`
	IsGameLoopRunning bool `json:"is_game_loop_running"`
}

func (handler *Handler) registerHealthHandlerSwagger(api *untyped.API) error {
	healthHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		res := HealthResponse{
			true, //see http://ismycomputeron.com/
			handler.w.IsGameLoopRunning()}
		return res, nil
	})
	api.RegisterOperation("GET", "/health", healthHandler)
	return nil
}
