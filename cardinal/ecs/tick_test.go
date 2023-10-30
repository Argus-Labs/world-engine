package ecs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/alicebob/miniredis/v2"
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"
	"pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

func TestTickHappyPath(t *testing.T) {
	rs := miniredis.RunT(t)
	oneWorld := testutil.InitWorldWithRedis(t, rs)
	assert.NilError(t, ecs.RegisterComponent[EnergyComponent](oneWorld))
	assert.NilError(t, oneWorld.LoadGameState())

	for i := 0; i < 10; i++ {
		assert.NilError(t, oneWorld.Tick(context.Background()))
	}

	assert.Equal(t, uint64(10), oneWorld.CurrentTick())

	twoWorld := testutil.InitWorldWithRedis(t, rs)
	assert.NilError(t, ecs.RegisterComponent[EnergyComponent](twoWorld))
	assert.NilError(t, twoWorld.LoadGameState())
	assert.Equal(t, uint64(10), twoWorld.CurrentTick())
}
func TestIfPanicMessageLogged(t *testing.T) {
	w := ecs.NewTestWorld(t)
	// replaces internal Logger with one that logs to the buf variable above.
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	cardinalLogger := log.Logger{
		&bufLogger,
	}
	w.InjectLogger(&cardinalLogger)
	// In this test, our "buggy" system fails once Power reaches 3
	errorTxt := "BIG ERROR OH NO"
	w.AddSystem(func(ecs.WorldContext) error {
		panic(errorTxt)
	})
	assert.NilError(t, w.LoadGameState())
	ctx := context.Background()

	defer func() {
		if panicValue := recover(); panicValue != nil {
			// This test should swallow a panic
			lastjson, err := findLastJSON(buf.Bytes())
			assert.NilError(t, err)
			values := map[string]string{}
			err = json.Unmarshal(lastjson, &values)
			assert.NilError(t, err)
			msg, ok := values["message"]
			assert.Assert(t, ok)
			assert.Equal(t, msg, "Tick: 0, Current running system: ecs_test.TestIfPanicMessageLogged.func1")
			panicString, ok := panicValue.(string)
			assert.Assert(t, ok)
			assert.Equal(t, panicString, errorTxt)
		} else {
			assert.Assert(t, false) // This test should create a panic.
		}
	}()

	err := w.Tick(ctx)
	assert.NilError(t, err)
}

func findLastJSON(buf []byte) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(buf))
	var lastVal json.RawMessage
	for {
		if err := dec.Decode(&lastVal); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
	}
	if lastVal == nil {
		return nil, fmt.Errorf("no JSON value found")
	}
	return lastVal, nil
}

type onePowerComponent struct {
	Power int
}

func (onePowerComponent) Name() string {
	return "onePower"
}

type twoPowerComponent struct {
	Power int
}

func (twoPowerComponent) Name() string {
	return "twoPower"
}

func TestCanIdentifyAndFixSystemError(t *testing.T) {
	rs := miniredis.RunT(t)
	oneWorld := testutil.InitWorldWithRedis(t, rs)
	assert.NilError(t, ecs.RegisterComponent[onePowerComponent](oneWorld))

	id, err := component.Create(ecs.NewWorldContext(oneWorld), onePowerComponent{})
	assert.NilError(t, err)

	errorSystem := errors.New("3 power? That's too much, man")

	// In this test, our "buggy" system fails once Power reaches 3
	oneWorld.AddSystem(func(wCtx ecs.WorldContext) error {
		var p *onePowerComponent
		p, err = component.GetComponent[onePowerComponent](wCtx, id)
		if err != nil {
			return err
		}
		p.Power++
		if p.Power >= 3 {
			return errorSystem
		}
		return component.SetComponent[onePowerComponent](wCtx, id, p)
	})
	assert.NilError(t, oneWorld.LoadGameState())

	// Power is set to 1
	assert.NilError(t, oneWorld.Tick(context.Background()))
	// Power is set to 2
	assert.NilError(t, oneWorld.Tick(context.Background()))
	// Power is set to 3, then the System fails
	assert.ErrorIs(t, errorSystem, oneWorld.Tick(context.Background()))

	// Set up a new world using the same storage layer
	twoWorld := testutil.InitWorldWithRedis(t, rs)
	assert.NilError(t, ecs.RegisterComponent[onePowerComponent](twoWorld))
	assert.NilError(t, ecs.RegisterComponent[twoPowerComponent](twoWorld))

	// this is our fixed system that can handle Power levels of 3 and higher
	twoWorld.AddSystem(func(wCtx ecs.WorldContext) error {
		var p *onePowerComponent
		p, err = component.GetComponent[onePowerComponent](wCtx, id)
		if err != nil {
			return err
		}
		p.Power++
		return component.SetComponent[onePowerComponent](wCtx, id, p)
	})

	// Loading a game state with the fixed system should automatically finish the previous tick.
	assert.NilError(t, twoWorld.LoadGameState())
	twoWorldCtx := ecs.NewWorldContext(twoWorld)
	p, err := component.GetComponent[onePowerComponent](twoWorldCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 3, p.Power)

	// Just for fun, tick one last time to make sure power is still being incremented.
	assert.NilError(t, twoWorld.Tick(context.Background()))
	p1, err := component.GetComponent[onePowerComponent](twoWorldCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 4, p1.Power)
}

type ScalarComponentAlpha struct {
	Val int
}

type ScalarComponentBeta struct {
	Val int
}

func (ScalarComponentAlpha) Name() string {
	return "alpha"
}

func (ScalarComponentBeta) Name() string {
	return "beta"
}

func TestCanModifyArchetypeAndGetEntity(t *testing.T) {
	world := ecs.NewTestWorld(t)
	assert.NilError(t, ecs.RegisterComponent[ScalarComponentAlpha](world))
	assert.NilError(t, ecs.RegisterComponent[ScalarComponentBeta](world))
	assert.NilError(t, world.LoadGameState())

	wCtx := ecs.NewWorldContext(world)
	wantID, err := component.Create(wCtx, ScalarComponentAlpha{})
	assert.NilError(t, err)

	wantScalar := ScalarComponentAlpha{99}

	assert.NilError(t, component.SetComponent[ScalarComponentAlpha](wCtx, wantID, &wantScalar))

	verifyCanFindEntity := func() {
		// Make sure we can find the entity
		var q *ecs.Search
		q, err = world.NewSearch(ecs.Contains(ScalarComponentAlpha{}))
		assert.NilError(t, err)
		gotID, err := q.First(wCtx)
		assert.NilError(t, err)
		assert.Equal(t, wantID, gotID)

		// Make sure the associated component is correct
		gotScalar, err := component.GetComponent[ScalarComponentAlpha](wCtx, wantID)
		assert.NilError(t, err)
		assert.Equal(t, wantScalar, *gotScalar)
	}

	// Make sure we can find the one-and-only entity ID
	verifyCanFindEntity()

	// Add on the beta component
	assert.NilError(t, component.AddComponentTo[Beta](wCtx, wantID))
	verifyCanFindEntity()

	// Remove the beta component
	assert.NilError(t, component.RemoveComponentFrom[Beta](wCtx, wantID))
	verifyCanFindEntity()
}

type ScalarComponentStatic struct {
	Val int
}

type ScalarComponentToggle struct {
	Val int
}

func (ScalarComponentStatic) Name() string {
	return "static"
}

func (ScalarComponentToggle) Name() string {
	return "toggle"
}

func TestCanRecoverStateAfterFailedArchetypeChange(t *testing.T) {
	rs := miniredis.RunT(t)
	for _, firstWorldIteration := range []bool{true, false} {
		world := testutil.InitWorldWithRedis(t, rs)
		assert.NilError(t, ecs.RegisterComponent[ScalarComponentStatic](world))
		assert.NilError(t, ecs.RegisterComponent[ScalarComponentToggle](world))

		wCtx := ecs.NewWorldContext(world)
		if firstWorldIteration {
			_, err := component.Create(wCtx, ScalarComponentStatic{})
			assert.NilError(t, err)
		}

		errorToggleComponent := errors.New("problem with toggle component")
		world.AddSystem(func(wCtx ecs.WorldContext) error {
			// Get the one and only entity ID
			q, err := wCtx.NewSearch(ecs.Contains(ScalarComponentStatic{}))
			assert.NilError(t, err)
			id, err := q.First(wCtx)
			assert.NilError(t, err)

			s, err := component.GetComponent[ScalarComponentStatic](wCtx, id)
			assert.NilError(t, err)
			s.Val++
			assert.NilError(t, component.SetComponent[ScalarComponentStatic](wCtx, id, s))
			if s.Val%2 == 1 {
				assert.NilError(t, component.AddComponentTo[ScalarComponentToggle](wCtx, id))
			} else {
				assert.NilError(t, component.RemoveComponentFrom[ScalarComponentToggle](wCtx, id))
			}

			if firstWorldIteration && s.Val == 5 {
				return errorToggleComponent
			}

			return nil
		})
		assert.NilError(t, world.LoadGameState())
		q, err := world.NewSearch(ecs.Contains(ScalarComponentStatic{}))
		assert.NilError(t, err)
		id, err := q.First(wCtx)
		assert.NilError(t, err)

		if firstWorldIteration {
			for i := 0; i < 4; i++ {
				assert.NilError(t, world.Tick(context.Background()))
			}
			// After 4 ticks, static.Val should be 4 and toggle should have just been removed from the entity.
			_, err = component.GetComponent[ScalarComponentToggle](wCtx, id)
			assert.ErrorIs(t, storage.ErrComponentNotOnEntity, err)

			// Ticking again should result in an error
			assert.ErrorIs(t, errorToggleComponent, world.Tick(context.Background()))
		} else {
			// At this second iteration, the errorToggleComponent bug has been fixed. static.Val should be 5
			// and toggle should have just been added to the entity.
			_, err = component.GetComponent[ScalarComponentToggle](wCtx, id)
			assert.NilError(t, err)

			var s *ScalarComponentStatic
			s, err = component.GetComponent[ScalarComponentStatic](wCtx, id)

			assert.NilError(t, err)
			assert.Equal(t, 5, s.Val)
		}
	}
}

type PowerComp struct {
	Val float64
}

func (PowerComp) Name() string {
	return "powerComp"
}

func TestCanRecoverTransactionsFromFailedSystemRun(t *testing.T) {
	rs := miniredis.RunT(t)
	errorBadPowerChange := errors.New("bad power change transaction")
	for _, isBuggyIteration := range []bool{true, false} {
		world := testutil.InitWorldWithRedis(t, rs)

		assert.NilError(t, ecs.RegisterComponent[PowerComp](world))

		powerTx := ecs.NewTransactionType[PowerComp, PowerComp]("change_power")
		assert.NilError(t, world.RegisterTransactions(powerTx))

		world.AddSystem(func(wCtx ecs.WorldContext) error {
			q, err := wCtx.NewSearch(ecs.Contains(PowerComp{}))
			assert.NilError(t, err)
			id := q.MustFirst(wCtx)
			entityPower, err := component.GetComponent[PowerComp](wCtx, id)
			assert.NilError(t, err)

			changes := powerTx.In(wCtx)
			assert.Equal(t, 1, len(changes))
			entityPower.Val += changes[0].Value.Val
			assert.NilError(t, component.SetComponent[PowerComp](wCtx, id, entityPower))

			if isBuggyIteration && changes[0].Value.Val == 666 {
				return errorBadPowerChange
			}
			return nil
		})
		assert.NilError(t, world.LoadGameState())

		wCtx := ecs.NewWorldContext(world)
		// Only create the entity for the first iteration
		if isBuggyIteration {
			_, err := component.Create(wCtx, PowerComp{})
			assert.NilError(t, err)
		}

		// fetchPower is a small helper to get the power of the only entity in the world
		fetchPower := func() float64 {
			q, err := world.NewSearch(ecs.Contains(PowerComp{}))
			assert.NilError(t, err)
			id, err := q.First(wCtx)
			assert.NilError(t, err)
			power, err := component.GetComponent[PowerComp](wCtx, id)
			assert.NilError(t, err)
			return power.Val
		}

		if isBuggyIteration {
			// perform a few ticks that will not result in an error
			powerTx.AddToQueue(world, PowerComp{1000})
			assert.NilError(t, world.Tick(context.Background()))
			powerTx.AddToQueue(world, PowerComp{1000})
			assert.NilError(t, world.Tick(context.Background()))
			powerTx.AddToQueue(world, PowerComp{1000})
			assert.NilError(t, world.Tick(context.Background()))

			assert.Equal(t, float64(3000), fetchPower())

			// In this "buggy" iteration, the above system cannot handle a power of 666.
			powerTx.AddToQueue(world, PowerComp{666})
			assert.ErrorIs(t, errorBadPowerChange, world.Tick(context.Background()))
		} else {
			// Loading the game state above should successfully re-process that final 666 transactions.
			assert.Equal(t, float64(3666), fetchPower())

			// One more tick for good measure
			powerTx.AddToQueue(world, PowerComp{1000})
			assert.NilError(t, world.Tick(context.Background()))

			assert.Equal(t, float64(4666), fetchPower())
		}
	}
}
