package counter_test

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

type Alpha struct{}

func (Alpha) Name() string { return "alpha" }

type Beta struct{}

func (Beta) Name() string { return "beta" }

type Gamma struct{}

func (Gamma) Name() string { return "gamma" }

func TestCounterWithEventHub(t *testing.T) {
	world, _ := testutils.MakeWorldAndTicker(t)
	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))
	assert.NilError(t, cardinal.RegisterComponent[Gamma](world))

	worldCtx := testutils.WorldToWorldContext(world)
	_, err := cardinal.CreateMany(worldCtx, 10, Alpha{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Beta{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Gamma{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Alpha{}, Beta{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Alpha{}, Gamma{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Beta{}, Gamma{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Alpha{}, Beta{}, Gamma{})
	assert.NilError(t, err)
}
