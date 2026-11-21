package physics2d_test

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// clashingTransform reuses physics2d's "transform_2d" component name with a different Go type, so a
// world that already registered it rejects the plugin's own component registration.
type clashingTransform struct {
	X int `json:"x"`
}

func (clashingTransform) Name() string { return "transform_2d" }

func (c clashingTransform) MarshalWire() []byte {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}

	return b
}

func (clashingTransform) UnmarshalWire(b []byte) (any, error) {
	var v clashingTransform

	err := json.Unmarshal(b, &v)

	return v, err
}

func (c clashingTransform) SizeWire() int { return len(c.MarshalWire()) }

func (c clashingTransform) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

// TestRegister_RejectedRegistrationLeavesInstanceUnregistered: a world that rejects the plugin's
// systems must leave the instance unregistered, so the same instance still registers on a fresh
// world instead of being refused as "called twice".
func TestRegister_RejectedRegistrationLeavesInstanceUnregistered(t *testing.T) {
	p := physics.NewPlugin(physics.Config{})

	clashing := newWorld(t)
	clashing.RegisterComponent[clashingTransform]()

	err := recoverPanic(func() { clashing.RegisterPlugin(p) })
	require.ErrorContains(t, err, "component transform_2d already registered with a different type")

	fresh := newWorld(t)
	require.NoError(t, recoverPanic(func() { fresh.RegisterPlugin(p) }))
	initCardinalECS(fresh)
	tickN(t, fresh, 1)
	require.NotNil(t, p.Engine(), "the plugin must be live on the world that accepted it")
}

// TestRegister_StepsAtWorldTickRate: physics time must follow the world's tick rate after a
// deployment's CARDINAL_TICK_RATE overrides the compiled 60 Hz, so one second of ticks moves a
// 6 m/s body 6 m at any deployed rate.
func TestRegister_StepsAtWorldTickRate(t *testing.T) {
	for _, rate := range []int{30, 60, 120} {
		t.Run(strconv.Itoa(rate)+"Hz", func(t *testing.T) {
			t.Setenv("CARDINAL_TICK_RATE", strconv.Itoa(rate))

			// The world and the plugin both compile in 60 Hz.
			w, _ := makeWorld(t, physics.Vec2{})

			var id cardinal.EntityID

			w.RegisterSystem(&spawnMoverSystem{run: func(w *cardinal.World) {
				row := w.Create[spawnArchetype]()
				row.Set(physics.Velocity2D{Linear: physics.Vec2{X: 6}})
				row.Set(newRigid(physics.BodyTypeKinematic, circleColliderShapes()...))
				id = row.ID()
			}}, cardinal.WithPhase(cardinal.Init))

			initCardinalECS(w)
			tickN(t, w, rate)

			got := w.Entity(id).Get[physics.Transform2D]().Position.X
			require.InDelta(t, 6, got, 1e-9, "position after %d ticks at %d Hz", rate, rate)
		})
	}
}

type spawnMoverSystem struct{ run func(w *cardinal.World) }

func (s *spawnMoverSystem) Run(w *cardinal.World) { s.run(w) }

// recoverPanic runs fn and returns the value it panicked with as an error, or nil when it returned.
func recoverPanic(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			if err, ok = r.(error); !ok {
				err = fmt.Errorf("%v", r)
			}
		}
	}()

	fn()

	return nil
}
