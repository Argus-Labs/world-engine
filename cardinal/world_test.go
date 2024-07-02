package cardinal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/worldstage"
)

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
	miniRedis := miniredis.RunT(t)
	t.Setenv("REDIS_ADDRESS", miniRedis.Addr())

	ctx := context.Background()

	for _, isFirstIteration := range []bool{true, false} {
		world, err := NewWorld(WithPort(getOpenPort(t)))
		assert.NilError(t, err)

		assert.NilError(t, RegisterComponent[ScalarComponentStatic](world))
		assert.NilError(t, RegisterComponent[ScalarComponentToggle](world))

		wCtx := NewWorldContext(world)

		errorToggleComponent := errors.New("problem with toggle component")
		err = RegisterSystems(
			world,
			func(wCtx WorldContext) error {
				// Get the one and only entity ID
				q := NewSearch().Entity(filter.Contains(filter.Component[ScalarComponentStatic]()))
				id, err := q.First(wCtx)
				assert.NilError(t, err)

				s, err := GetComponent[ScalarComponentStatic](wCtx, id)
				assert.NilError(t, err)

				s.Val++
				assert.NilError(t, SetComponent[ScalarComponentStatic](wCtx, id, s))

				if s.Val%2 == 1 {
					assert.NilError(t, AddComponentTo[ScalarComponentToggle](wCtx, id))
				} else {
					assert.NilError(t, RemoveComponentFrom[ScalarComponentToggle](wCtx, id))
				}

				if isFirstIteration && s.Val == 5 {
					return errorToggleComponent
				}

				return nil
			},
		)
		assert.NilError(t, err)

		go func() {
			err = world.StartGame()
			assert.NilError(t, err)
		}()
		<-world.worldStage.NotifyOnStage(worldstage.Running)

		if isFirstIteration {
			_, err := Create(wCtx, ScalarComponentStatic{})
			assert.NilError(t, err)
		}
		q := NewSearch().Entity(filter.Contains(filter.Component[ScalarComponentStatic]()))
		id, err := q.First(wCtx)
		assert.NilError(t, err)

		if isFirstIteration {
			for i := 0; i < 4; i++ {
				world.tickTheEngine(ctx, nil)
			}
			// After 4 ticks, static.Val should be 4 and toggle should have just been removed from the entity.
			_, err = GetComponent[ScalarComponentToggle](wCtx, id)
			assert.ErrorIs(t, iterators.ErrComponentNotOnEntity, eris.Cause(err))

			s, err := GetComponent[ScalarComponentStatic](wCtx, id)
			assert.NilError(t, err)
			assert.Equal(t, 4, s.Val)

			// Ticking again should result in an error
			err = doTickCapturePanic(ctx, world)
			assert.ErrorContains(t, err, errorToggleComponent.Error())
		} else {
			// At this second iteration, the errorToggleComponent bug has been fixed.
			// It should recover at the last successful tick where toggle does not exist on the entity and val is 4
			_, err = GetComponent[ScalarComponentToggle](wCtx, id)
			assert.ErrorIs(t, iterators.ErrComponentNotOnEntity, eris.Cause(err))

			s, err := GetComponent[ScalarComponentStatic](wCtx, id)
			assert.NilError(t, err)
			assert.Equal(t, 4, s.Val)

			world.tickTheEngine(ctx, nil)

			// After ticking again, static.Val should be 4 and toggle should have just been added to the entity.
			_, err = GetComponent[ScalarComponentToggle](wCtx, id)
			assert.NilError(t, err)

			s, err = GetComponent[ScalarComponentStatic](wCtx, id)
			assert.NilError(t, err)
			assert.Equal(t, 5, s.Val)
		}

		assert.NilError(t, world.Shutdown())
		CleanupViper(t)
	}

	miniRedis.Close()
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
	t.Setenv("REDIS_ADDRESS", rs.Addr())

	ctx := context.Background()

	world, err := NewWorld(WithPort(getOpenPort(t)))
	assert.NilError(t, err)
	defer CleanupViper(t)

	assert.NilError(t, RegisterComponent[onePowerComponent](world))

	errorSystem := errors.New("3 power? That's too much, man")

	// In this test, our "buggy" system fails once Power reaches 3
	err = RegisterSystems(
		world,
		func(wCtx WorldContext) error {
			searchObject := NewSearch().Entity(filter.Exact(filter.Component[onePowerComponent]()))
			id := searchObject.MustFirst(wCtx)
			p, err := GetComponent[onePowerComponent](wCtx, id)
			if err != nil {
				return err
			}
			p.Power++
			if p.Power >= 3 {
				return errorSystem
			}
			return SetComponent[onePowerComponent](wCtx, id, p)
		},
	)
	assert.NilError(t, err)

	go func() {
		err = world.StartGame()
		assert.NilError(t, err)
	}()
	<-world.worldStage.NotifyOnStage(worldstage.Running)

	id, err := Create(NewWorldContext(world), onePowerComponent{})
	assert.NilError(t, err)

	// Power is set to 1
	world.tickTheEngine(ctx, nil)
	// Power is set to 2
	world.tickTheEngine(ctx, nil)
	// Power is set to 3, then the system fails
	err = doTickCapturePanic(ctx, world)
	assert.ErrorContains(t, err, errorSystem.Error())

	assert.NilError(t, world.Shutdown())

	// Set up a new engine using the same storage layer
	world2, err := NewWorld(WithPort(getOpenPort(t)))
	assert.NilError(t, err)
	assert.NilError(t, RegisterComponent[onePowerComponent](world2))
	assert.NilError(t, RegisterComponent[twoPowerComponent](world2))

	// this is our fixed system that can handle Power levels of 3 and higher
	err = RegisterSystems(
		world2,
		func(wCtx WorldContext) error {
			p, err := GetComponent[onePowerComponent](wCtx, id)
			if err != nil {
				return err
			}
			p.Power++
			return SetComponent[onePowerComponent](wCtx, id, p)
		},
	)
	assert.NilError(t, err)

	go func() {
		err = world2.StartGame()
		assert.NilError(t, err)
	}()
	<-world2.worldStage.NotifyOnStage(worldstage.Running)

	// Loading a game state with the fixed system should start back from the last successful tick.
	world2Ctx := NewWorldContext(world2)
	world2.tickTheEngine(ctx, nil)
	p1, err := GetComponent[onePowerComponent](world2Ctx, id)
	assert.NilError(t, err)
	assert.Equal(t, 3, p1.Power)
	assert.NilError(t, world2.Shutdown())
}

type Foo struct{}

func (Foo) Name() string { return "foo" }

type Bar struct{}

func (Bar) Name() string { return "bar" }

type Qux struct{}

func (Qux) Name() string { return "qux" }

// TestSystemsPanicOnRedisError ensures systems panic when there is a problem connecting to redis. In general, Systems
// should panic on ANY fatal error, but this connection problem is how we'll simulate a non ecs state related error.
func TestSystemsPanicOnRedisError(t *testing.T) {
	testCases := []struct {
		name string
		// the failFn will be called at a time when the ECB is empty of cached data and redis is down.
		failFn func(wCtx WorldContext, goodID types.EntityID)
	}{
		{
			name: "AddComponentTo",
			failFn: func(wCtx WorldContext, goodID types.EntityID) {
				_ = AddComponentTo[Qux](wCtx, goodID)
			},
		},
		{
			name: "RemoveComponentFrom",
			failFn: func(wCtx WorldContext, goodID types.EntityID) {
				_ = RemoveComponentFrom[Bar](wCtx, goodID)
			},
		},
		{
			name: "GetComponent",
			failFn: func(wCtx WorldContext, goodID types.EntityID) {
				_, _ = GetComponent[Foo](wCtx, goodID)
			},
		},
		{
			name: "SetComponent",
			failFn: func(wCtx WorldContext, goodID types.EntityID) {
				_ = SetComponent[Foo](wCtx, goodID, &Foo{})
			},
		},
		{
			name: "UpdateComponent",
			failFn: func(wCtx WorldContext, goodID types.EntityID) {
				_ = UpdateComponent[Foo](wCtx, goodID, func(f *Foo) *Foo {
					return f
				})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			miniRedis := miniredis.RunT(t)
			t.Setenv("REDIS_ADDRESS", miniRedis.Addr())

			ctx := context.Background()

			world, err := NewWorld(WithPort(getOpenPort(t)))
			assert.NilError(t, err)
			defer CleanupViper(t)

			assert.NilError(t, RegisterComponent[Foo](world))
			assert.NilError(t, RegisterComponent[Bar](world))
			assert.NilError(t, RegisterComponent[Qux](world))

			// This system will be called 2 times. The first time, a single entity is Created. The second time,
			// the previously Created entity is fetched, and then miniRedis is closed. Subsequent attempts to access
			// data should panic.
			assert.NilError(t, RegisterSystems(world, func(wCtx WorldContext) error {
				// Set up the entity in the first tick
				if wCtx.CurrentTick() == 0 {
					_, err := Create(wCtx, Foo{}, Bar{})
					assert.Check(t, err == nil)
					return nil
				}
				// Get the valid entity for the second tick
				id, err := NewSearch().Entity(filter.Exact(filter.Component[Foo](),
					filter.Component[Bar]())).First(wCtx)
				assert.Check(t, err == nil)
				assert.Check(t, id != iterators.BadID)

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

			go func() {
				err = world.StartGame()
				assert.NilError(t, err)
			}()
			<-world.worldStage.NotifyOnStage(worldstage.Running)
			defer func() {
				assert.NilError(t, world.Shutdown())
			}()

			// The first tick sets up the entity
			world.tickTheEngine(ctx, nil)
			// The second tick calls the test case's failure function.
			err = doTickCapturePanic(ctx, world)
			assert.IsError(t, err)
		})
	}
}

func doTickCapturePanic(ctx context.Context, world *World) (err error) {
	defer func() {
		if panicValue := recover(); panicValue != nil {
			err = fmt.Errorf(panicValue.(string))
		}
	}()
	world.tickTheEngine(ctx, nil)

	return nil
}

func getOpenPort(t testing.TB) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	defer func() {
		assert.NilError(t, l.Close())
	}()

	assert.NilError(t, err)
	tcpAddr, err := net.ResolveTCPAddr(l.Addr().Network(), l.Addr().String())
	assert.NilError(t, err)
	return fmt.Sprintf("%d", tcpAddr.Port)
}
