package server_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

func TestDebugEndpoint(t *testing.T) {
	world := testutils.NewTestWorld(t).Instance()

	assert.NilError(t, ecs.RegisterComponent[Alpha](world))
	assert.NilError(t, ecs.RegisterComponent[Beta](world))
	assert.NilError(t, ecs.RegisterComponent[Gamma](world))

	assert.NilError(t, world.LoadGameState())
	ctx := context.Background()
	worldCtx := ecs.NewWorldContext(world)
	_, err := component.CreateMany(worldCtx, 10, Alpha{})
	assert.NilError(t, err)
	_, err = component.CreateMany(worldCtx, 10, Beta{})
	assert.NilError(t, err)
	_, err = component.CreateMany(worldCtx, 10, Gamma{})
	assert.NilError(t, err)
	_, err = component.CreateMany(worldCtx, 10, Alpha{}, Beta{})
	assert.NilError(t, err)
	_, err = component.CreateMany(worldCtx, 10, Alpha{}, Gamma{})
	assert.NilError(t, err)
	_, err = component.CreateMany(worldCtx, 10, Beta{}, Gamma{})
	assert.NilError(t, err)
	_, err = component.CreateMany(worldCtx, 10, Alpha{}, Beta{}, Gamma{})
	assert.NilError(t, err)
	err = world.Tick(ctx)
	assert.NilError(t, err)
	txh := testutils.MakeTestTransactionHandler(t, world, server.DisableSignatureVerification())
	resp := txh.Get("debug/state")
	assert.Equal(t, resp.StatusCode, 200)
	bz, err := io.ReadAll(resp.Body)
	assert.NilError(t, err)
	data := make([]json.RawMessage, 0)
	err = json.Unmarshal(bz, &data)
	assert.NilError(t, err)
	assert.Equal(t, len(data), 10*7)
}

func TestDebugEndpointMustAccessReadOnlyData(t *testing.T) {
	world := testutils.NewTestWorld(t).Instance()

	// midTickCh is used to ensure the /debug/state call starts and ends in the middle of a System tick.
	midTickCh := make(chan struct{})

	assert.NilError(t, ecs.RegisterComponent[Delta](world))
	var targetID entity.ID
	world.RegisterSystem(func(worldCtx ecs.WorldContext) error {
		// This system increments Delta.Value by 50 twice. /debug/state should see Delta.Value = 0 OR Delta.Value = 100,
		// But never Delta.Value = 50.
		assert.Check(t, nil == component.UpdateComponent[Delta](worldCtx, targetID, func(d *Delta) *Delta {
			d.Value += 50
			return d
		}))
		<-midTickCh
		<-midTickCh
		assert.Check(t, nil == component.UpdateComponent[Delta](worldCtx, targetID, func(d *Delta) *Delta {
			d.Value += 50
			return d
		}))
		return nil
	})

	assert.NilError(t, world.LoadGameState())
	worldCtx := ecs.NewWorldContext(world)
	var err error
	targetID, err = component.Create(worldCtx, Delta{})
	assert.NilError(t, err)

	go func() {
		// Ignore errors from these ticks. This tests is focused on making sure we're reading from the write places.
		ctx := context.Background()
		// Tick one: Make sure the entity is created
		_ = world.Tick(ctx)
		// Tick two: Call /debug/state mid-tick and verify Delta.Value is 100
		_ = world.Tick(ctx)
		// Tick three: Call /debug/state mid-tick and verify Delta.Value is 200
		_ = world.Tick(ctx)
	}()

	// Don't check anything for the first tick.
	midTickCh <- struct{}{}
	midTickCh <- struct{}{}

	txh := testutils.MakeTestTransactionHandler(t, world, server.DisableSignatureVerification())
	getDeltaValue := func() int {
		resp := txh.Get("debug/state")
		assert.Equal(t, resp.StatusCode, 200)
		var data []struct {
			Id   int
			Data []Delta
		}
		err = json.NewDecoder(resp.Body).Decode(&data)
		assert.NilError(t, err)
		assert.Equal(t, len(data), 1)
		return data[0].Data[0].Value
	}

	// Pause in the middle of the second tick
	midTickCh <- struct{}{}
	assert.Equal(t, 100, getDeltaValue())
	// Let the second tick finish
	midTickCh <- struct{}{}
	// Pause in the middle of the third tick
	midTickCh <- struct{}{}
	assert.Equal(t, 200, getDeltaValue())
	// Let the third tick finish
	midTickCh <- struct{}{}
}
