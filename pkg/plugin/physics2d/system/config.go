package system

import (
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// RuntimeConfig is simulation parameters for reconcile/step systems, set from physics2d.Plugin.Register.
//
//nolint:gochecknoglobals // set once at plugin registration
var runtimeConfig = defaultRuntimeConfig()

// defaultRuntimeConfig returns Box2D-friendly defaults (60 Hz, common iteration counts).
func defaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		FixedDT:            1.0 / 60.0,
		VelocityIterations: 8,
		PositionIterations: 3,
	}
}

// RuntimeConfig holds gravity and stepping parameters passed to Box2D.
type RuntimeConfig struct {
	Gravity            component.Vec2
	FixedDT            float64
	VelocityIterations int
	PositionIterations int
}

// SetRuntimeConfig stores config for physics systems; call from Plugin.Register only.
// Zero FixedDT or iteration counts are replaced with defaults.
func SetRuntimeConfig(c RuntimeConfig) {
	runtimeConfig = c
	if runtimeConfig.FixedDT <= 0 {
		runtimeConfig.FixedDT = defaultRuntimeConfig().FixedDT
	}
	if runtimeConfig.VelocityIterations <= 0 {
		runtimeConfig.VelocityIterations = defaultRuntimeConfig().VelocityIterations
	}
	if runtimeConfig.PositionIterations <= 0 {
		runtimeConfig.PositionIterations = defaultRuntimeConfig().PositionIterations
	}
}

func stepConfig() RuntimeConfig {
	return runtimeConfig
}
