package server1

import (
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware/untyped"
)

type HealthReply struct {
	IsServerRunning   bool `json:"isServerRunning"`
	IsGameLoopRunning bool `json:"isGameLoopRunning"`
}

func (handler *Handler) registerHealthHandlerSwagger(api *untyped.API) {
	healthHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		res := HealthReply{
			true, // see http://ismycomputeron.com/
			handler.w.IsGameLoopRunning()}
		return res, nil
	})
	api.RegisterOperation("GET", "/health", healthHandler)
}
