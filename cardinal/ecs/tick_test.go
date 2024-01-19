package ecs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/iterators"

	"testing"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"github.com/alicebob/miniredis/v2"
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"
)

func TestTickHappyPath(t *testing.T) {
	rs := miniredis.RunT(t)
	oneEngine := testutil.InitEngineWithRedis(t, rs)
	assert.NilError(t, ecs.RegisterComponent[EnergyComponent](oneEngine))
	assert.NilError(t, oneEngine.LoadGameState())

	for i := 0; i < 10; i++ {
		assert.NilError(t, oneEngine.Tick(context.Background()))
	}

	assert.Equal(t, uint64(10), oneEngine.CurrentTick())

	twoEngine := testutil.InitEngineWithRedis(t, rs)
	assert.NilError(t, ecs.RegisterComponent[EnergyComponent](twoEngine))
	assert.NilError(t, twoEngine.LoadGameState())
	assert.Equal(t, uint64(10), twoEngine.CurrentTick())
}
func TestIfPanicMessageLogged(t *testing.T) {
	engine := testutils.NewTestWorld(t).Engine()
	// replaces internal Logger with one that logs to the buf variable above.
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	engine.InjectLogger(&bufLogger)
	// In this test, our "buggy" system fails once Power reaches 3
	errorTxt := "BIG ERROR OH NO"
	engine.RegisterSystem(
		func(ecs.EngineContext) error {
			panic(errorTxt)
		},
	)
	assert.NilError(t, engine.LoadGameState())
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

	err := engine.Tick(ctx)
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
	oneEngine := testutil.InitEngineWithRedis(t, rs)
	assert.NilError(t, ecs.RegisterComponent[onePowerComponent](oneEngine))

	errorSystem := errors.New("3 power? That's too much, man")

	// In this test, our "buggy" system fails once Power reaches 3
	oneEngine.RegisterSystem(
		func(eCtx ecs.EngineContext) error {
			search := eCtx.NewSearch(filter.Exact(onePowerComponent{}))
			id := search.MustFirst(eCtx)
			p, err := ecs.GetComponent[onePowerComponent](eCtx, id)
			if err != nil {
				return err
			}
			p.Power++
			if p.Power >= 3 {
				return errorSystem
			}
			return ecs.SetComponent[onePowerComponent](eCtx, id, p)
		},
	)
	assert.NilError(t, oneEngine.LoadGameState())
	id, err := ecs.Create(ecs.NewEngineContext(oneEngine), onePowerComponent{})
	assert.NilError(t, err)

	// Power is set to 1
	assert.NilError(t, oneEngine.Tick(context.Background()))
	// Power is set to 2
	assert.NilError(t, oneEngine.Tick(context.Background()))
	// Power is set to 3, then the System fails
	assert.ErrorIs(t, errorSystem, eris.Cause(oneEngine.Tick(context.Background())))

	// Set up a new engine using the same storage layer
	twoEngine := testutil.InitEngineWithRedis(t, rs)
	assert.NilError(t, ecs.RegisterComponent[onePowerComponent](twoEngine))
	assert.NilError(t, ecs.RegisterComponent[twoPowerComponent](twoEngine))

	// this is our fixed system that can handle Power levels of 3 and higher
	twoEngine.RegisterSystem(
		func(eCtx ecs.EngineContext) error {
			p, err := ecs.GetComponent[onePowerComponent](eCtx, id)
			if err != nil {
				return err
			}
			p.Power++
			return ecs.SetComponent[onePowerComponent](eCtx, id, p)
		},
	)

	// Loading a game state with the fixed system should automatically finish the previous tick.
	assert.NilError(t, twoEngine.LoadGameState())
	twoEngineCtx := ecs.NewEngineContext(twoEngine)
	p, err := ecs.GetComponent[onePowerComponent](twoEngineCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 3, p.Power)

	// Just for fun, tick one last time to make sure power is still being incremented.
	assert.NilError(t, twoEngine.Tick(context.Background()))
	p1, err := ecs.GetComponent[onePowerComponent](twoEngineCtx, id)
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
	engine := testutils.NewTestWorld(t).Engine()
	assert.NilError(t, ecs.RegisterComponent[ScalarComponentAlpha](engine))
	assert.NilError(t, ecs.RegisterComponent[ScalarComponentBeta](engine))
	assert.NilError(t, engine.LoadGameState())

	eCtx := ecs.NewEngineContext(engine)
	wantID, err := ecs.Create(eCtx, ScalarComponentAlpha{})
	assert.NilError(t, err)

	wantScalar := ScalarComponentAlpha{99}

	assert.NilError(t, ecs.SetComponent[ScalarComponentAlpha](eCtx, wantID, &wantScalar))

	verifyCanFindEntity := func() {
		// Make sure we can find the entity
		q := engine.NewSearch(filter.Contains(ScalarComponentAlpha{}))
		gotID, err := q.First(eCtx)
		assert.NilError(t, err)
		assert.Equal(t, wantID, gotID)

		// Make sure the associated component is correct
		gotScalar, err := ecs.GetComponent[ScalarComponentAlpha](eCtx, wantID)
		assert.NilError(t, err)
		assert.Equal(t, wantScalar, *gotScalar)
	}

	// Make sure we can find the one-and-only entity ID
	verifyCanFindEntity()

	// Add on the beta component
	assert.NilError(t, ecs.AddComponentTo[Beta](eCtx, wantID))
	verifyCanFindEntity()

	// Remove the beta component
	assert.NilError(t, ecs.RemoveComponentFrom[Beta](eCtx, wantID))
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
	for _, firstEngineIteration := range []bool{true, false} {
		engine := testutil.InitEngineWithRedis(t, rs)
		assert.NilError(t, ecs.RegisterComponent[ScalarComponentStatic](engine))
		assert.NilError(t, ecs.RegisterComponent[ScalarComponentToggle](engine))

		eCtx := ecs.NewEngineContext(engine)

		errorToggleComponent := errors.New("problem with toggle component")
		engine.RegisterSystem(
			func(eCtx ecs.EngineContext) error {
				// Get the one and only entity ID
				q := eCtx.NewSearch(filter.Contains(ScalarComponentStatic{}))
				id, err := q.First(eCtx)
				assert.NilError(t, err)

				s, err := ecs.GetComponent[ScalarComponentStatic](eCtx, id)
				assert.NilError(t, err)
				s.Val++
				assert.NilError(t, ecs.SetComponent[ScalarComponentStatic](eCtx, id, s))
				if s.Val%2 == 1 {
					assert.NilError(t, ecs.AddComponentTo[ScalarComponentToggle](eCtx, id))
				} else {
					assert.NilError(t, ecs.RemoveComponentFrom[ScalarComponentToggle](eCtx, id))
				}

				if firstEngineIteration && s.Val == 5 {
					return errorToggleComponent
				}

				return nil
			},
		)
		assert.NilError(t, engine.LoadGameState())
		if firstEngineIteration {
			_, err := ecs.Create(eCtx, ScalarComponentStatic{})
			assert.NilError(t, err)
		}
		q := engine.NewSearch(filter.Contains(ScalarComponentStatic{}))
		id, err := q.First(eCtx)
		assert.NilError(t, err)

		if firstEngineIteration {
			for i := 0; i < 4; i++ {
				assert.NilError(t, engine.Tick(context.Background()))
			}
			// After 4 ticks, static.Val should be 4 and toggle should have just been removed from the entity.
			_, err = ecs.GetComponent[ScalarComponentToggle](eCtx, id)
			assert.ErrorIs(t, iterators.ErrComponentNotOnEntity, eris.Cause(err))

			// Ticking again should result in an error
			assert.ErrorIs(t, errorToggleComponent, eris.Cause(engine.Tick(context.Background())))
		} else {
			// At this second iteration, the errorToggleComponent bug has been fixed. static.Val should be 5
			// and toggle should have just been added to the entity.
			_, err = ecs.GetComponent[ScalarComponentToggle](eCtx, id)
			assert.NilError(t, err)

			s, err := ecs.GetComponent[ScalarComponentStatic](eCtx, id)

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
	errorBadPowerChange := errors.New("bad power change message")
	for _, isBuggyIteration := range []bool{true, false} {
		engine := testutil.InitEngineWithRedis(t, rs)

		assert.NilError(t, ecs.RegisterComponent[PowerComp](engine))

		powerTx := ecs.NewMessageType[PowerComp, PowerComp]("change_power")
		assert.NilError(t, engine.RegisterMessages(powerTx))

		engine.RegisterSystem(
			func(eCtx ecs.EngineContext) error {
				q := eCtx.NewSearch(filter.Contains(PowerComp{}))
				id := q.MustFirst(eCtx)
				entityPower, err := ecs.GetComponent[PowerComp](eCtx, id)
				assert.NilError(t, err)

				changes := powerTx.In(eCtx)
				assert.Equal(t, 1, len(changes))
				entityPower.Val += changes[0].Msg.Val
				assert.NilError(t, ecs.SetComponent[PowerComp](eCtx, id, entityPower))

				if isBuggyIteration && changes[0].Msg.Val == 666 {
					return errorBadPowerChange
				}
				return nil
			},
		)
		assert.NilError(t, engine.LoadGameState())

		eCtx := ecs.NewEngineContext(engine)
		// Only create the entity for the first iteration
		if isBuggyIteration {
			_, err := ecs.Create(eCtx, PowerComp{})
			assert.NilError(t, err)
		}

		// fetchPower is a small helper to get the power of the only entity in the engine
		fetchPower := func() float64 {
			q := engine.NewSearch(filter.Contains(PowerComp{}))
			id, err := q.First(eCtx)
			assert.NilError(t, err)
			power, err := ecs.GetComponent[PowerComp](eCtx, id)
			assert.NilError(t, err)
			return power.Val
		}

		if isBuggyIteration {
			// perform a few ticks that will not result in an error
			powerTx.AddToQueue(engine, PowerComp{1000})
			assert.NilError(t, engine.Tick(context.Background()))
			powerTx.AddToQueue(engine, PowerComp{1000})
			assert.NilError(t, engine.Tick(context.Background()))
			powerTx.AddToQueue(engine, PowerComp{1000})
			assert.NilError(t, engine.Tick(context.Background()))

			assert.Equal(t, float64(3000), fetchPower())

			// In this "buggy" iteration, the above system cannot handle a power of 666.
			powerTx.AddToQueue(engine, PowerComp{666})
			assert.ErrorIs(t, errorBadPowerChange, eris.Cause(engine.Tick(context.Background())))
		} else {
			// Loading the game state above should successfully re-process that final 666 messages.
			assert.Equal(t, float64(3666), fetchPower())

			// One more tick for good measure
			powerTx.AddToQueue(engine, PowerComp{1000})
			assert.NilError(t, engine.Tick(context.Background()))

			assert.Equal(t, float64(4666), fetchPower())
		}
	}
}
