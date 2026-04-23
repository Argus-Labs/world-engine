package system

import (
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// RuntimeConfig is simulation parameters for reconcile/step systems, set from physics2d.Plugin.Register.
//
//nolint:gochecknoglobals // set once at plugin registration
var runtimeConfig = defaultRuntimeConfig()

// defaultRuntimeConfig returns Box2D v3 friendly defaults (60 Hz, 4 sub-steps).
func defaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		FixedDT:      1.0 / 60.0,
		SubStepCount: 4,
	}
}

// RuntimeConfig holds gravity and stepping parameters passed to the physics simulation.
type RuntimeConfig struct {
	Gravity      component.Vec2
	FixedDT      float64
	SubStepCount int
}

// SetRuntimeConfig stores config for physics systems; call from Plugin.Register only.
// Zero FixedDT or SubStepCount are replaced with defaults.
func SetRuntimeConfig(c RuntimeConfig) {
	runtimeConfig = c
	if runtimeConfig.FixedDT <= 0 {
		runtimeConfig.FixedDT = defaultRuntimeConfig().FixedDT
	}
	if runtimeConfig.SubStepCount <= 0 {
		runtimeConfig.SubStepCount = defaultRuntimeConfig().SubStepCount
	}
}

func stepConfig() RuntimeConfig {
	return runtimeConfig
}
