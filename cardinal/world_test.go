package cardinal_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/search"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/sign"
)

func TestIfPanicMessageLogged(t *testing.T) {
	// cardinal.Create a logger that writes to a buffer so we can check the output
	buf := &bytes.Buffer{}
	bufLogger := zerolog.New(buf)

	tf := testutils.NewTestFixture(t, nil, cardinal.WithCustomLogger(bufLogger))
	world := tf.World

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

	// Should panic and return an error
	_, err = tf.DoTick()
	assert.NotNil(t, err)

	t.Log(buf.String())

	assert.NilError(t, findGameLoopError(buf))
}

type ScalarComponentStatic struct {
	TickCounter int
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

func TestWorld_CanRecoverStateAfterFailedArchetypeChange(t *testing.T) {
	mr := miniredis.RunT(t)
	t.Setenv("CARDINAL_LOG_LEVEL", "debug")

	for iteration := range 2 {
		tf := testutils.NewTestFixture(t, mr)
		world := tf.World
		t.Logf("Starting iteration %d", iteration)

		assert.NilError(t, cardinal.RegisterComponent[ScalarComponentStatic](world))
		assert.NilError(t, cardinal.RegisterComponent[ScalarComponentToggle](world))

		errorToggleComponent := errors.New("problem with toggle component")

		// Register the main test system
		err := cardinal.RegisterSystems(world, func(wCtx engine.Context) error {
			// Get the one and only entity ID
			q := cardinal.NewSearch(wCtx, filter.Contains(ScalarComponentStatic{}))
			id, err := q.First()
			assert.NilError(t, err)

			s, err := cardinal.GetComponent[ScalarComponentStatic](wCtx, id)
			assert.NilError(t, err)

			s.TickCounter++

			assert.NilError(t, cardinal.SetComponent[ScalarComponentStatic](wCtx, id, s))

			if s.TickCounter%2 == 1 {
				assert.NilError(t, cardinal.AddComponentTo[ScalarComponentToggle](wCtx, id))
			} else {
				assert.NilError(t, cardinal.RemoveComponentFrom[ScalarComponentToggle](wCtx, id))
			}

			// On the first iteration, on tick 5, return an error from the system to trigger a panic.
			if iteration == 0 && s.TickCounter == 5 {
				return errorToggleComponent
			}

			return nil
		})
		assert.NilError(t, err)

		// Start the world
		tf.StartWorld()

		// Create a world context for the test
		wCtx := cardinal.NewWorldContext(world)

		if iteration == 0 {
			// On the first iteration, the system has an issue and panics on tick 4

			// Create an entity on tick 0
			_, err = cardinal.Create(wCtx, ScalarComponentStatic{})
			assert.NilError(t, err)

			// Search for static entity
			s := cardinal.NewSearch(wCtx, filter.Contains(ScalarComponentStatic{}))
			id, err := s.First()
			assert.NilError(t, err)

			// Tick until tick 3 is reached
			for i := 0; i < 4; i++ {
				_, err = tf.DoTick()
				assert.NilError(t, err)
			}

			// After tick 3, toggle should have just been removed from the entity.
			_, err = cardinal.GetComponent[ScalarComponentToggle](wCtx, id)
			assert.ErrorIs(t, cardinal.ErrComponentNotOnEntity, eris.Cause(err))

			// After tick 3, ScalarComponentStatic.TickCounter should be 4
			staticComp, err := cardinal.GetComponent[ScalarComponentStatic](wCtx, id)
			assert.NilError(t, err)
			assert.Equal(t, 4, staticComp.TickCounter)

			// Ticking again should result in an error because tick counter is now 5
			_, err = tf.DoTick()
			assert.ErrorContains(t, err, errorToggleComponent.Error())
		} else {
			// At this second iteration, the error has been fixed.
			// The system should recover and not panic on tick 4.

			// Search for static entity
			s := cardinal.NewSearch(wCtx, filter.Contains(ScalarComponentStatic{}))
			id, err := s.First()
			assert.NilError(t, err)

			// After tick 4, toggle should have just been added to the entity.
			_, err = cardinal.GetComponent[ScalarComponentToggle](wCtx, id)
			assert.NilError(t, err)

			// After tick 4, ScalarComponentStatic.TickCounter should be 5
			staticComp, err := cardinal.GetComponent[ScalarComponentStatic](wCtx, id)
			assert.NilError(t, err)
			assert.Equal(t, 5, staticComp.TickCounter)
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
	mr := miniredis.RunT(t)

	errorBadPowerChange := errors.New("bad power change message")
	for _, isBuggyIteration := range []bool{true, false} {
		tf := testutils.NewTestFixture(t, mr)
		world := tf.World

		assert.NilError(t, cardinal.RegisterComponent[PowerComp](world))
		msgName := "change_power"
		assert.NilError(t, cardinal.RegisterMessage[PowerComp, PowerComp](world, msgName))

		err := cardinal.RegisterSystems(
			world,
			func(wCtx engine.Context) error {
				q := cardinal.NewSearch(wCtx, filter.Contains(PowerComp{}))
				id := q.MustFirst()
				entityPower, err := cardinal.GetComponent[PowerComp](wCtx, id)
				assert.NilError(t, err)
				changes := make([]message.TxData[PowerComp], 0)
				err = cardinal.EachMessage[PowerComp, PowerComp](wCtx,
					func(msg message.TxData[PowerComp]) (PowerComp, error) {
						changes = append(changes, msg)
						return msg.Msg, nil
					})
				assert.NilError(t, err)
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
		// Only create the entity for the first iteration
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
		powerTx, ok := world.GetMessageByFullName("game." + msgName)
		if isBuggyIteration {
			// perform a few ticks that will not result in an error
			assert.True(t, ok)
			world.AddTransaction(powerTx.ID(), PowerComp{1000}, &sign.Transaction{})
			_, err = tf.DoTick()
			assert.NilError(t, err)
			world.AddTransaction(powerTx.ID(), PowerComp{1000}, &sign.Transaction{})
			_, err = tf.DoTick()
			assert.NilError(t, err)
			world.AddTransaction(powerTx.ID(), PowerComp{1000}, &sign.Transaction{})
			_, err = tf.DoTick()
			assert.NilError(t, err)
			assert.Equal(t, float64(3000), fetchPower())

			// In this "buggy" iteration, the above system cannot handle a power of 666.
			world.AddTransaction(powerTx.ID(), PowerComp{666}, &sign.Transaction{})
			_, err = tf.DoTick()
			assert.ErrorContains(t, err, errorBadPowerChange.Error())
		} else {
			// Loading the game state above should successfully re-process that final 666 messages.
			assert.Equal(t, float64(3666), fetchPower())

			// One more tick for good measure
			world.AddTransaction(powerTx.ID(), PowerComp{1000}, &sign.Transaction{})
			_, err = tf.DoTick()
			assert.NilError(t, err)

			assert.Equal(t, float64(4666), fetchPower())
		}
		tf.Shutdown()
	}
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
	mr := miniredis.RunT(t)
	tf := testutils.NewTestFixture(t, mr)
	world := tf.World

	assert.NilError(t, cardinal.RegisterComponent[onePowerComponent](world))

	errorSystem := errors.New("3 power? That's too much, man")

	// In this test, our "buggy" system fails once Power reaches 3
	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			s := cardinal.NewSearch(wCtx, filter.Exact(onePowerComponent{}))
			id := s.MustFirst()
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

	tf.StartWorld()

	id, err := cardinal.Create(cardinal.NewWorldContext(world), onePowerComponent{})
	assert.NilError(t, err)

	// Power is set to 1
	_, err = tf.DoTick()
	assert.NilError(t, err)
	// Power is set to 2
	_, err = tf.DoTick()
	assert.NilError(t, err)
	// Power is set to 3, then the System fails
	_, err = tf.DoTick()
	assert.ErrorContains(t, err, errorSystem.Error())

	assert.NilError(t, world.Shutdown())

	// Set up a new engine using the same storage layer
	tf2 := testutils.NewTestFixture(t, mr)
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
	_, err = tf2.DoTick()
	assert.NilError(t, err)
	p1, err := cardinal.GetComponent[onePowerComponent](world2Ctx, id)
	assert.NilError(t, err)
	assert.Equal(t, 4, p1.Power)

	assert.NilError(t, world2.Shutdown())
}

// TestSystemsPanicOnRedisError ensures systems panic when there is a problem connecting to redis. In general, Systems
// should panic on ANY fatal error, but this connection problem is how we'll simulate a non ecs state related error.

func TestSystemsPanicOnRedisError(t *testing.T) {
	testCases := []struct {
		name string
		// the failFn will be called at a time when the ECB is empty of cached data and redis is down.
		failFn func(wCtx engine.Context, goodID types.EntityID)
	}{
		{
			name: "cardinal.AddComponentTo",
			failFn: func(wCtx engine.Context, goodID types.EntityID) {
				_ = cardinal.AddComponentTo[Qux](wCtx, goodID)
			},
		},
		{
			name: "cardinal.RemoveComponentFrom",
			failFn: func(wCtx engine.Context, goodID types.EntityID) {
				_ = cardinal.RemoveComponentFrom[Bar](wCtx, goodID)
			},
		},
		{
			name: "cardinal.GetComponent",
			failFn: func(wCtx engine.Context, goodID types.EntityID) {
				_, _ = cardinal.GetComponent[Foo](wCtx, goodID)
			},
		},
		{
			name: "cardinal.SetComponent",
			failFn: func(wCtx engine.Context, goodID types.EntityID) {
				_ = cardinal.SetComponent[Foo](wCtx, goodID, &Foo{})
			},
		},
		{
			name: "cardinal.UpdateComponent",
			failFn: func(wCtx engine.Context, goodID types.EntityID) {
				_ = cardinal.UpdateComponent[Foo](wCtx, goodID, func(f *Foo) *Foo {
					return f
				})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			miniRedis := miniredis.RunT(t)
			tf := testutils.NewTestFixture(t, miniRedis)
			world := tf.World

			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			assert.NilError(t, cardinal.RegisterComponent[Bar](world))
			assert.NilError(t, cardinal.RegisterComponent[Qux](world))

			// This system will be called 2 times. The first time, a single entity is Created. The second time,
			// the previously Created entity is fetched, and then miniRedis is closed. Subsequent attempts to access
			// data should panic.
			assert.NilError(t, cardinal.RegisterSystems(world, func(wCtx engine.Context) error {
				// Set up the entity in the first tick
				if wCtx.CurrentTick() == 0 {
					_, err := cardinal.Create(wCtx, Foo{}, Bar{})
					assert.Check(t, err == nil)
					return nil
				}
				// Get the valid entity for the second tick
				id, err := cardinal.NewSearch(wCtx, filter.Exact(Foo{}, Bar{})).First()
				assert.Check(t, err == nil)
				assert.Check(t, id != search.BadID)

				// Shut down redis. The testCase's failure function will now be able to fail
				miniRedis.Close()

				// Only set up this panic/recover expectation if we're in the second tick.
				defer func() {
					err := recover()
					assert.Check(t, err != nil, "expected panic")
				}()

				tc.failFn(wCtx, id)
				assert.Check(t, false, "should never reach here")
				return nil
			}))

			tf.StartWorld()

			// The first tick sets up the entity
			_, err := tf.DoTick()
			assert.NilError(t, err)

			// The second tick calls the test case's failure function.
			_, err = tf.DoTick()
			assert.IsError(t, err)
		})
	}
}

func findGameLoopError(buf *bytes.Buffer) error {
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		// Try decoding log line to JSON
		// If we can't decode the line, just skip it
		var logLine map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &logLine); err != nil {
			continue
		}

		// If we found the game loop error, return nil error.
		if logLine["message"] == "error occured during game loop" {
			return nil
		}
	}
	return eris.New("game loop error not found")
}
