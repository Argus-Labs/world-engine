package cardinal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"io"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"testing"
	"time"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"github.com/alicebob/miniredis/v2"
)

func TestTickHappyPath(t *testing.T) {
	rs := miniredis.RunT(t)
	tf1 := testutils.NewTestFixture(t, rs)
	world1 := tf1.World
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponent](world1))
	tf1.StartWorld()

	for i := 0; i < 10; i++ {
		assert.NilError(t, world1.Tick(context.Background(), uint64(time.Now().Unix())))
	}

	assert.Equal(t, uint64(10), world1.CurrentTick())

	tf2 := testutils.NewTestFixture(t, rs)
	world2 := tf2.World
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponent](world2))
	tf2.StartWorld()
	assert.Equal(t, uint64(10), world2.CurrentTick())
}
func TestIfPanicMessageLogged(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
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
	tf.StartWorld()
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

	err = world.Tick(ctx, uint64(time.Now().Unix()))
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
	tf1 := testutils.NewTestFixture(t, rs)
	world1 := tf1.World
	assert.NilError(t, cardinal.RegisterComponent[onePowerComponent](world1))

	errorSystem := errors.New("3 power? That's too much, man")

	// In this test, our "buggy" system fails once Power reaches 3
	err := cardinal.RegisterSystems(
		world1,
		func(wCtx engine.Context) error {
			search := cardinal.NewSearch(wCtx, filter.Exact(onePowerComponent{}))
			id := search.MustFirst()
			p, err := cardinal.GetComponent[onePowerComponent](wCtx, id)
			if err != nil {
				return err
			}
			p.Power++
			if p.Power >= 3 {
				return errorSystem
			}
			return cardinal.SetComponent[onePowerComponent](wCtx, id, p)
		},
	)
	assert.NilError(t, err)
	tf1.StartWorld()
	id, err := cardinal.Create(cardinal.NewWorldContext(world1), onePowerComponent{})
	assert.NilError(t, err)

	// Power is set to 1
	assert.NilError(t, world1.Tick(context.Background(), uint64(time.Now().Unix())))
	// Power is set to 2
	assert.NilError(t, world1.Tick(context.Background(), uint64(time.Now().Unix())))
	// Power is set to 3, then the System fails
	assert.ErrorIs(t, errorSystem, eris.Cause(world1.Tick(context.Background(), uint64(time.Now().Unix()))))

	// Set up a new engine using the same storage layer
	tf2 := testutils.NewTestFixture(t, rs)
	world2 := tf2.World
	assert.NilError(t, cardinal.RegisterComponent[onePowerComponent](world2))
	assert.NilError(t, cardinal.RegisterComponent[twoPowerComponent](world2))

	// this is our fixed system that can handle Power levels of 3 and higher
	err = cardinal.RegisterSystems(
		world2,
		func(wCtx engine.Context) error {
			p, err := cardinal.GetComponent[onePowerComponent](wCtx, id)
			if err != nil {
				return err
			}
			p.Power++
			return cardinal.SetComponent[onePowerComponent](wCtx, id, p)
		},
	)
	assert.NilError(t, err)

	// Loading a game state with the fixed system should automatically finish the previous tick.
	tf2.StartWorld()
	world2Ctx := cardinal.NewWorldContext(world2)
	p, err := cardinal.GetComponent[onePowerComponent](world2Ctx, id)
	assert.NilError(t, err)
	assert.Equal(t, 3, p.Power)

	// Just for fun, tick one last time to make sure power is still being incremented.
	assert.NilError(t, world2.Tick(context.Background(), uint64(time.Now().Unix())))
	p1, err := cardinal.GetComponent[onePowerComponent](world2Ctx, id)
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[ScalarComponentAlpha](world))
	assert.NilError(t, cardinal.RegisterComponent[ScalarComponentBeta](world))
	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	wantID, err := cardinal.Create(wCtx, ScalarComponentAlpha{})
	assert.NilError(t, err)

	wantScalar := ScalarComponentAlpha{99}

	assert.NilError(t, cardinal.SetComponent[ScalarComponentAlpha](wCtx, wantID, &wantScalar))

	verifyCanFindEntity := func() {
		// Make sure we can find the entity
		q := cardinal.NewSearch(wCtx, filter.Contains(ScalarComponentAlpha{}))
		gotID, err := q.First()
		assert.NilError(t, err)
		assert.Equal(t, wantID, gotID)

		// Make sure the associated component is correct
		gotScalar, err := cardinal.GetComponent[ScalarComponentAlpha](wCtx, wantID)
		assert.NilError(t, err)
		assert.Equal(t, wantScalar, *gotScalar)
	}

	// Make sure we can find the one-and-only entity ID
	verifyCanFindEntity()

	// Add on the beta component
	assert.NilError(t, cardinal.AddComponentTo[Beta](wCtx, wantID))
	verifyCanFindEntity()

	// Remove the beta component
	assert.NilError(t, cardinal.RemoveComponentFrom[Beta](wCtx, wantID))
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
		tf := testutils.NewTestFixture(t, rs)
		world := tf.World
		assert.NilError(t, cardinal.RegisterComponent[ScalarComponentStatic](world))
		assert.NilError(t, cardinal.RegisterComponent[ScalarComponentToggle](world))

		wCtx := cardinal.NewWorldContext(world)

		errorToggleComponent := errors.New("problem with toggle component")
		err := cardinal.RegisterSystems(
			world,
			func(wCtx engine.Context) error {
				// Get the one and only entity ID
				q := cardinal.NewSearch(wCtx, filter.Contains(ScalarComponentStatic{}))
				id, err := q.First()
				assert.NilError(t, err)

				s, err := cardinal.GetComponent[ScalarComponentStatic](wCtx, id)
				assert.NilError(t, err)
				s.Val++
				assert.NilError(t, cardinal.SetComponent[ScalarComponentStatic](wCtx, id, s))
				if s.Val%2 == 1 {
					assert.NilError(t, cardinal.AddComponentTo[ScalarComponentToggle](wCtx, id))
				} else {
					assert.NilError(t, cardinal.RemoveComponentFrom[ScalarComponentToggle](wCtx, id))
				}

				if firstEngineIteration && s.Val == 5 {
					return errorToggleComponent
				}

				return nil
			},
		)
		assert.NilError(t, err)
		tf.StartWorld()

		if firstEngineIteration {
			_, err := cardinal.Create(wCtx, ScalarComponentStatic{})
			assert.NilError(t, err)
		}
		q := cardinal.NewSearch(wCtx, filter.Contains(ScalarComponentStatic{}))
		id, err := q.First()
		assert.NilError(t, err)

		if firstEngineIteration {
			for i := 0; i < 4; i++ {
				tf.StartTickCh <- time.Now()
				<-tf.DoneTickCh
			}
			// After 4 ticks, static.Val should be 4 and toggle should have just been removed from the entity.
			_, err = cardinal.GetComponent[ScalarComponentToggle](wCtx, id)
			assert.ErrorIs(t, iterators.ErrComponentNotOnEntity, eris.Cause(err))

			// Ticking again should result in an error
			assert.ErrorIs(t, errorToggleComponent,
				eris.Cause(tf.World.Tick(context.Background(), uint64(time.Now().Unix()))))
		} else {
			// At this second iteration, the errorToggleComponent bug has been fixed. static.Val should be 5
			// and toggle should have just been added to the entity.
			_, err = cardinal.GetComponent[ScalarComponentToggle](wCtx, id)
			assert.NilError(t, err)

			s, err := cardinal.GetComponent[ScalarComponentStatic](wCtx, id)

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
		tf := testutils.NewTestFixture(t, rs)
		world := tf.World

		assert.NilError(t, cardinal.RegisterComponent[PowerComp](world))

		powerTx := message.NewMessageType[PowerComp, PowerComp]("change_power")
		assert.NilError(t, cardinal.RegisterMessages(world, powerTx))

		err := cardinal.RegisterSystems(
			world,
			func(wCtx engine.Context) error {
				q := cardinal.NewSearch(wCtx, filter.Contains(PowerComp{}))
				id := q.MustFirst()
				entityPower, err := cardinal.GetComponent[PowerComp](wCtx, id)
				assert.NilError(t, err)

				changes := powerTx.In(wCtx)
				assert.Equal(t, 1, len(changes))
				entityPower.Val += changes[0].Msg.Val
				assert.NilError(t, cardinal.SetComponent[PowerComp](wCtx, id, entityPower))

				if isBuggyIteration && changes[0].Msg.Val == 666 {
					return errorBadPowerChange
				}
				return nil
			},
		)
		assert.NilError(t, err)
		tf.StartWorld()

		wCtx := cardinal.NewWorldContext(world)
		// Only cardinal.Create the entity for the first iteration
		if isBuggyIteration {
			_, err := cardinal.Create(wCtx, PowerComp{})
			assert.NilError(t, err)
		}

		// fetchPower is a small helper to get the power of the only entity in the engine
		fetchPower := func() float64 {
			q := cardinal.NewSearch(wCtx, filter.Contains(PowerComp{}))
			id, err := q.First()
			assert.NilError(t, err)
			power, err := cardinal.GetComponent[PowerComp](wCtx, id)
			assert.NilError(t, err)
			return power.Val
		}

		if isBuggyIteration {
			// perform a few ticks that will not result in an error
			tf.AddTransaction(powerTx.ID(), PowerComp{1000})
			assert.NilError(t, world.Tick(context.Background(), uint64(time.Now().Unix())))
			tf.AddTransaction(powerTx.ID(), PowerComp{1000})
			assert.NilError(t, world.Tick(context.Background(), uint64(time.Now().Unix())))
			tf.AddTransaction(powerTx.ID(), PowerComp{1000})
			assert.NilError(t, world.Tick(context.Background(), uint64(time.Now().Unix())))

			assert.Equal(t, float64(3000), fetchPower())

			// In this "buggy" iteration, the above system cannot handle a power of 666.
			tf.AddTransaction(powerTx.ID(), PowerComp{666})
			assert.ErrorIs(t, errorBadPowerChange,
				eris.Cause(world.Tick(context.Background(), uint64(time.Now().Unix()))))
		} else {
			// Loading the game state above should successfully re-process that final 666 messages.
			assert.Equal(t, float64(3666), fetchPower())

			// One more tick for good measure
			tf.AddTransaction(powerTx.ID(), PowerComp{1000})
			tf.DoTick()

			assert.Equal(t, float64(4666), fetchPower())
		}
	}
}
