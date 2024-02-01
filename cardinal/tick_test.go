package cardinal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rs/zerolog"
	"io"
	"pkg.world.dev/world-engine/cardinal"
	filter2 "pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/types/engine"

	"testing"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"github.com/alicebob/miniredis/v2"
)

func TestTickHappyPath(t *testing.T) {
	rs := miniredis.RunT(t)
	oneWorld := testutils.NewTestFixture(t, rs).World
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponent](oneWorld))
	assert.NilError(t, oneWorld.LoadGameState())

	for i := 0; i < 10; i++ {
		assert.NilError(t, oneWorld.Tick(context.Background()))
	}

	assert.Equal(t, uint64(10), oneWorld.CurrentTick())

	twoWorld := testutils.NewTestFixture(t, rs).World
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponent](twoWorld))
	assert.NilError(t, twoWorld.LoadGameState())
	assert.Equal(t, uint64(10), twoWorld.CurrentTick())
}
func TestIfPanicMessageLogged(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	// replaces internal Logger with one that logs to the buf variable above.
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	world.InjectLogger(&bufLogger)
	// In this test, our "buggy" system fails once Power reaches 3
	errorTxt := "BIG ERROR OH NO"
	err := cardinal.RegisterSystems(
		world,
		func(engine.Context) error {
			panic(errorTxt)
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, world.LoadGameState())
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
			assert.Equal(t, msg, "Tick: 0, Current running system: cardinal_test.TestIfPanicMessageLogged.func1")
			panicString, ok := panicValue.(string)
			assert.Assert(t, ok)
			assert.Equal(t, panicString, errorTxt)
		} else {
			assert.Assert(t, false) // This test should create a panic.
		}
	}()

	err = world.Tick(ctx)
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
	oneWorld := testutils.NewTestFixture(t, rs).World
	assert.NilError(t, cardinal.RegisterComponent[onePowerComponent](oneWorld))

	errorSystem := errors.New("3 power? That's too much, man")

	// In this test, our "buggy" system fails once Power reaches 3
	err := cardinal.RegisterSystems(
		oneWorld,
		func(eCtx engine.Context) error {
			search := cardinal.NewSearch(eCtx, filter2.Exact(onePowerComponent{}))
			id := search.MustFirst()
			p, err := cardinal.GetComponent[onePowerComponent](eCtx, id)
			if err != nil {
				return err
			}
			p.Power++
			if p.Power >= 3 {
				return errorSystem
			}
			return cardinal.SetComponent[onePowerComponent](eCtx, id, p)
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, oneWorld.LoadGameState())
	id, err := cardinal.Create(cardinal.NewWorldContext(oneWorld), onePowerComponent{})
	assert.NilError(t, err)

	// Power is set to 1
	assert.NilError(t, oneWorld.Tick(context.Background()))
	// Power is set to 2
	assert.NilError(t, oneWorld.Tick(context.Background()))
	// Power is set to 3, then the System fails
	assert.ErrorIs(t, errorSystem, eris.Cause(oneWorld.Tick(context.Background())))

	// Set up a new engine using the same storage layer
	twoWorld := testutils.NewTestFixture(t, rs).World
	assert.NilError(t, cardinal.RegisterComponent[onePowerComponent](twoWorld))
	assert.NilError(t, cardinal.RegisterComponent[twoPowerComponent](twoWorld))

	// this is our fixed system that can handle Power levels of 3 and higher
	err = cardinal.RegisterSystems(
		twoWorld,
		func(eCtx engine.Context) error {
			p, err := cardinal.GetComponent[onePowerComponent](eCtx, id)
			if err != nil {
				return err
			}
			p.Power++
			return cardinal.SetComponent[onePowerComponent](eCtx, id, p)
		},
	)
	assert.NilError(t, err)

	// Loading a game state with the fixed system should automatically finish the previous tick.
	assert.NilError(t, twoWorld.LoadGameState())
	twoEngineCtx := cardinal.NewWorldContext(twoWorld)
	p, err := cardinal.GetComponent[onePowerComponent](twoEngineCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 3, p.Power)

	// Just for fun, tick one last time to make sure power is still being incremented.
	assert.NilError(t, twoWorld.Tick(context.Background()))
	p1, err := cardinal.GetComponent[onePowerComponent](twoEngineCtx, id)
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
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, cardinal.RegisterComponent[ScalarComponentAlpha](world))
	assert.NilError(t, cardinal.RegisterComponent[ScalarComponentBeta](world))
	assert.NilError(t, world.LoadGameState())

	eCtx := cardinal.NewWorldContext(world)
	wantID, err := cardinal.Create(eCtx, ScalarComponentAlpha{})
	assert.NilError(t, err)

	wantScalar := ScalarComponentAlpha{99}

	assert.NilError(t, cardinal.SetComponent[ScalarComponentAlpha](eCtx, wantID, &wantScalar))

	verifyCanFindEntity := func() {
		// Make sure we can find the entity
		q := cardinal.NewSearch(eCtx, filter2.Contains(ScalarComponentAlpha{}))
		gotID, err := q.First()
		assert.NilError(t, err)
		assert.Equal(t, wantID, gotID)

		// Make sure the associated component is correct
		gotScalar, err := cardinal.GetComponent[ScalarComponentAlpha](eCtx, wantID)
		assert.NilError(t, err)
		assert.Equal(t, wantScalar, *gotScalar)
	}

	// Make sure we can find the one-and-only entity ID
	verifyCanFindEntity()

	// Add on the beta component
	assert.NilError(t, cardinal.AddComponentTo[Beta](eCtx, wantID))
	verifyCanFindEntity()

	// Remove the beta component
	assert.NilError(t, cardinal.RemoveComponentFrom[Beta](eCtx, wantID))
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
		world := testutils.NewTestFixture(t, rs).World
		assert.NilError(t, cardinal.RegisterComponent[ScalarComponentStatic](world))
		assert.NilError(t, cardinal.RegisterComponent[ScalarComponentToggle](world))

		eCtx := cardinal.NewWorldContext(world)

		errorToggleComponent := errors.New("problem with toggle component")
		err := cardinal.RegisterSystems(
			world,
			func(eCtx engine.Context) error {
				// Get the one and only entity ID
				q := cardinal.NewSearch(eCtx, filter2.Contains(ScalarComponentStatic{}))
				id, err := q.First()
				assert.NilError(t, err)

				s, err := cardinal.GetComponent[ScalarComponentStatic](eCtx, id)
				assert.NilError(t, err)
				s.Val++
				assert.NilError(t, cardinal.SetComponent[ScalarComponentStatic](eCtx, id, s))
				if s.Val%2 == 1 {
					assert.NilError(t, cardinal.AddComponentTo[ScalarComponentToggle](eCtx, id))
				} else {
					assert.NilError(t, cardinal.RemoveComponentFrom[ScalarComponentToggle](eCtx, id))
				}

				if firstEngineIteration && s.Val == 5 {
					return errorToggleComponent
				}

				return nil
			},
		)
		assert.NilError(t, err)
		assert.NilError(t, world.LoadGameState())
		if firstEngineIteration {
			_, err := cardinal.Create(eCtx, ScalarComponentStatic{})
			assert.NilError(t, err)
		}
		q := cardinal.NewSearch(eCtx, filter2.Contains(ScalarComponentStatic{}))
		id, err := q.First()
		assert.NilError(t, err)

		if firstEngineIteration {
			for i := 0; i < 4; i++ {
				assert.NilError(t, world.Tick(context.Background()))
			}
			// After 4 ticks, static.Val should be 4 and toggle should have just been removed from the entity.
			_, err = cardinal.GetComponent[ScalarComponentToggle](eCtx, id)
			assert.ErrorIs(t, iterators.ErrComponentNotOnEntity, eris.Cause(err))

			// Ticking again should result in an error
			assert.ErrorIs(t, errorToggleComponent, eris.Cause(world.Tick(context.Background())))
		} else {
			// At this second iteration, the errorToggleComponent bug has been fixed. static.Val should be 5
			// and toggle should have just been added to the entity.
			_, err = cardinal.GetComponent[ScalarComponentToggle](eCtx, id)
			assert.NilError(t, err)

			s, err := cardinal.GetComponent[ScalarComponentStatic](eCtx, id)

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
		world := testutils.NewTestFixture(t, rs).World

		assert.NilError(t, cardinal.RegisterComponent[PowerComp](world))

		powerTx := cardinal.NewMessageType[PowerComp, PowerComp]("change_power")
		assert.NilError(t, cardinal.RegisterMessages(world, powerTx))

		err := cardinal.RegisterSystems(
			world,
			func(eCtx engine.Context) error {
				q := cardinal.NewSearch(eCtx, filter2.Contains(PowerComp{}))
				id := q.MustFirst()
				entityPower, err := cardinal.GetComponent[PowerComp](eCtx, id)
				assert.NilError(t, err)

				changes := powerTx.In(eCtx)
				assert.Equal(t, 1, len(changes))
				entityPower.Val += changes[0].Msg.Val
				assert.NilError(t, cardinal.SetComponent[PowerComp](eCtx, id, entityPower))

				if isBuggyIteration && changes[0].Msg.Val == 666 {
					return errorBadPowerChange
				}
				return nil
			},
		)
		assert.NilError(t, err)
		assert.NilError(t, world.LoadGameState())

		eCtx := cardinal.NewWorldContext(world)
		// Only cardinal.Create the entity for the first iteration
		if isBuggyIteration {
			_, err := cardinal.Create(eCtx, PowerComp{})
			assert.NilError(t, err)
		}

		// fetchPower is a small helper to get the power of the only entity in the engine
		fetchPower := func() float64 {
			q := cardinal.NewSearch(eCtx, filter2.Contains(PowerComp{}))
			id, err := q.First()
			assert.NilError(t, err)
			power, err := cardinal.GetComponent[PowerComp](eCtx, id)
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
			assert.ErrorIs(t, errorBadPowerChange, eris.Cause(world.Tick(context.Background())))
		} else {
			// Loading the game state above should successfully re-process that final 666 messages.
			assert.Equal(t, float64(3666), fetchPower())

			// One more tick for good measure
			powerTx.AddToQueue(world, PowerComp{1000})
			assert.NilError(t, world.Tick(context.Background()))

			assert.Equal(t, float64(4666), fetchPower())
		}
	}
}
