package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

func TestDebugEndpoint(t *testing.T) {
	world := testutils.NewTestWorld(t).Instance()

	assert.NilError(t, ecs.RegisterComponent[Alpha](world))
	assert.NilError(t, ecs.RegisterComponent[Beta](world))
	assert.NilError(t, ecs.RegisterComponent[Gamma](world))

	assert.NilError(t, world.LoadGameState())
	ctx := context.Background()
	worldCtx := ecs.NewWorldContext(world)
	_, err := ecs.CreateMany(worldCtx, 10, Alpha{})
	assert.NilError(t, err)
	_, err = ecs.CreateMany(worldCtx, 10, Beta{})
	assert.NilError(t, err)
	_, err = ecs.CreateMany(worldCtx, 10, Gamma{})
	assert.NilError(t, err)
	_, err = ecs.CreateMany(worldCtx, 10, Alpha{}, Beta{})
	assert.NilError(t, err)
	_, err = ecs.CreateMany(worldCtx, 10, Alpha{}, Gamma{})
	assert.NilError(t, err)
	_, err = ecs.CreateMany(worldCtx, 10, Beta{}, Gamma{})
	assert.NilError(t, err)
	_, err = ecs.CreateMany(worldCtx, 10, Alpha{}, Beta{}, Gamma{})
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

func TestDebugAndCQLEndpointMustAccessReadOnlyData(t *testing.T) {
	world := testutils.NewTestWorld(t).Instance()

	// midTickCh is used to ensure the /debug/state call starts and ends in the middle of a System tick.
	midTickCh := make(chan struct{})

	assert.NilError(t, ecs.RegisterComponent[Delta](world))
	var targetID entity.ID
	world.RegisterSystem(
		func(worldCtx ecs.WorldContext) error {
			// This system increments Delta.Value by 50 twice. /debug/state should see Delta.Value = 0 OR Delta.Value = 100,
			// But never Delta.Value = 50.
			assert.Check(
				t, nil == ecs.UpdateComponent[Delta](
					worldCtx, targetID, func(d *Delta) *Delta {
						d.DeltaValue += 50
						return d
					},
				),
			)
			<-midTickCh
			<-midTickCh
			assert.Check(
				t, nil == ecs.UpdateComponent[Delta](
					worldCtx, targetID, func(d *Delta) *Delta {
						d.DeltaValue += 50
						return d
					},
				),
			)
			return nil
		},
	)

	assert.NilError(t, world.LoadGameState())
	worldCtx := ecs.NewWorldContext(world)
	var err error
	targetID, err = ecs.Create(worldCtx, Delta{})
	assert.NilError(t, err)

	startNextTick := make(chan struct{})
	defer func() {
		close(startNextTick)
	}()
	go func() {
		// Ignore errors from these ticks. This tests is focused on making sure we're reading from the write places.
		ctx := context.Background()
		// Tick one: Make sure the entity is created
		_ = world.Tick(ctx)
		for range startNextTick {
			_ = world.Tick(ctx)
		}
	}()

	// Don't check anything for the first tick.
	midTickCh <- struct{}{}
	midTickCh <- struct{}{}

	txh := testutils.MakeTestTransactionHandler(t, world, server.DisableSignatureVerification())
	defer txh.Close()
	testCases := []struct {
		name            string
		makeHTTPRequest func() *http.Response
	}{
		{
			name: "use /debug/state",
			makeHTTPRequest: func() *http.Response {
				return txh.Get("debug/state")
			},
		},
		{
			name: "use cql",
			makeHTTPRequest: func() *http.Response {
				return txh.Post(
					"query/game/cql", map[string]string{
						"CQL": "EXACT(delta)",
					},
				)
			},
		},
	}

	// This test assumes /debug/state and cql return data in the same format.
	for _, tc := range testCases {
		startNextTick <- struct{}{}
		midTickCh <- struct{}{}
		// We're now paused in the middle of the tick

		resp := tc.makeHTTPRequest()
		assert.Equal(t, 200, resp.StatusCode, tc.name)
		var data []struct {
			ID   int
			Data []Delta
		}
		err = json.NewDecoder(resp.Body).Decode(&data)
		assert.NilError(t, err, tc.name)
		assert.Equal(t, len(data), 1, tc.name)
		assert.Equal(t, len(data[0].Data), 1, tc.name)
		value := data[0].Data[0].DeltaValue
		// Each system increments Delta.Value by 50 two times, so value%100 should
		// always be 0. If it's ever 50, we know we're looking at mid-tick data.
		assert.Equal(t, 0, value%100, tc.name)

		// Allow the tick to complete
		midTickCh <- struct{}{}
	}
}
