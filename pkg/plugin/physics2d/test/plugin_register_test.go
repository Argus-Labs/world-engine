package physics2d_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// clashingTransform reuses physics2d's "transform_2d" component name with a different Go type, so a
// world that already registered it rejects the plugin's own systems.
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

type clashingSearch = cardinal.Exact[struct {
	T cardinal.WithComponent[clashingTransform]
}]

// TestRegister_RejectedRegistrationLeavesInstanceUnregistered: a world that rejects the plugin's
// systems must leave the instance unregistered, so the same instance still registers on a fresh
// world instead of being refused as "called twice".
func TestRegister_RejectedRegistrationLeavesInstanceUnregistered(t *testing.T) {
	p := physics.NewPlugin(physics.Config{})

	clashing := newWorld(t)
	clashing.RegisterSystem(func(*struct {
		cardinal.BaseSystemState
		Rows clashingSearch
	}) {
	})
	err := recoverPanic(func() { clashing.RegisterPlugin(p) })
	require.ErrorContains(t, err, "component transform_2d already registered with a different type")

	fresh := newWorld(t)
	require.NoError(t, recoverPanic(func() { fresh.RegisterPlugin(p) }))
	initCardinalECS(fresh)
	tickN(t, fresh, 1)
	require.NotNil(t, p.Engine(), "the plugin must be live on the world that accepted it")
}

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
