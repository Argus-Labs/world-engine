package cardinal_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

func TestWriteContextsReturnNonFatalErrors(t *testing.T) {
	const nonExistentEntityID = 999
	testCases := []struct {
		name    string
		testFn  func(cardinal.WorldContext) error
		wantErr error
	}{
		{
			name: "AddComponentTo_BadEntity",
			testFn: func(worldCtx cardinal.WorldContext) error {
				return cardinal.AddComponentTo[Foo](worldCtx, nonExistentEntityID)
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "AddComponentTo_ComponentAlreadyOnEntity",
			testFn: func(worldCtx cardinal.WorldContext) error {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.AddComponentTo[Foo](worldCtx, id)
			},
			wantErr: cardinal.ErrComponentAlreadyOnEntity,
		},
		{
			name: "RemoveComponentFrom_BadEntity",
			testFn: func(worldCtx cardinal.WorldContext) error {
				return cardinal.RemoveComponentFrom[Foo](worldCtx, nonExistentEntityID)
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "RemoveComponentFrom_ComponentNotOnEntity",
			testFn: func(worldCtx cardinal.WorldContext) error {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.RemoveComponentFrom[Bar](worldCtx, id)
			},
			wantErr: cardinal.ErrComponentNotOnEntity,
		},
		{
			name: "RemoveComponentFrom_EntityMustHaveAtLeastOneComponent",
			testFn: func(worldCtx cardinal.WorldContext) error {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.RemoveComponentFrom[Foo](worldCtx, id)
			},
			wantErr: cardinal.ErrEntityHasNoComponents,
		},
		{
			name: "GetComponent_BadEntity",
			testFn: func(worldCtx cardinal.WorldContext) error {
				_, err := cardinal.GetComponent[Foo](worldCtx, nonExistentEntityID)
				return err
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "GetComponent_ComponentNotOnEntity",
			testFn: func(worldCtx cardinal.WorldContext) error {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				_, err = cardinal.GetComponent[Bar](worldCtx, id)
				return err
			},
			wantErr: cardinal.ErrComponentNotOnEntity,
		},
		{
			name: "SetComponent_BadEntity",
			testFn: func(worldCtx cardinal.WorldContext) error {
				return cardinal.SetComponent[Foo](worldCtx, nonExistentEntityID, &Foo{})
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "SetComponent_ComponentNotOnEntity",
			testFn: func(worldCtx cardinal.WorldContext) error {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.SetComponent[Bar](worldCtx, id, &Bar{})
			},
			wantErr: cardinal.ErrComponentNotOnEntity,
		},
		{
			name: "UpdateComponent_BadEntity",
			testFn: func(worldCtx cardinal.WorldContext) error {
				return cardinal.UpdateComponent[Foo](worldCtx, nonExistentEntityID, func(f *Foo) *Foo {
					return f
				})
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "UpdateComponent_ComponentNotOnEntity",
			testFn: func(worldCtx cardinal.WorldContext) error {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.UpdateComponent[Bar](worldCtx, id, func(b *Bar) *Bar {
					return b
				})
			},
			wantErr: cardinal.ErrComponentNotOnEntity,
		},
		{
			name: "Remove_EntityDoesNotExist",
			testFn: func(worldCtx cardinal.WorldContext) error {
				return cardinal.Remove(worldCtx, nonExistentEntityID)
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			world, tick := testutils.MakeWorldAndTicker(t)
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			assert.NilError(t, cardinal.RegisterComponent[Bar](world))
			world.Init(func(worldCtx cardinal.WorldContext) error {
				defer func() {
					// In Systems, Cardinal is designed to panic when a fatal error is encountered.
					// This test is not supposed to panic, but if it does it happens in a non-main thread which
					// makes it hard to track down where the panic actually came from.
					// Recover here and complain about any non-nil panics to allow the remaining tests in this
					// function to be executed.
					err := recover()
					assert.Check(t, err == nil, "got fatal error \"%v\"", err)
				}()

				err := tc.testFn(worldCtx)
				isWantError := errors.Is(err, tc.wantErr)
				assert.Check(t, isWantError, "expected %v but got %v", tc.wantErr, err)
				return nil
			})
			tick()
		})
	}
}

type UnregisteredComp struct{}

func (UnregisteredComp) Name() string { return "unregistered_comp" }

// TestWriteContextsFailWhenComponentHasNotBeenRegistered ensures that state mutating method that encounter a component
// that has not been registered will panic.
func TestWriteContextsPanicOnComponentHasNotBeenRegistered(t *testing.T) {
	testCases := []struct {
		name string
		// Every test is expected to panic, so no return error is needed
		testFn func(cardinal.WorldContext)
	}{
		{
			name: "AddComponentTo",
			testFn: func(worldCtx cardinal.WorldContext) {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				_ = cardinal.AddComponentTo[UnregisteredComp](worldCtx, id)
			},
		},
		{
			name: "RemoveComponentFrom",
			testFn: func(worldCtx cardinal.WorldContext) {
				id, err := cardinal.Create(worldCtx, Foo{}, Bar{})
				assert.Check(t, err == nil)
				_ = cardinal.RemoveComponentFrom[UnregisteredComp](worldCtx, id)
			},
		},
		{
			name: "GetComponent",
			testFn: func(worldCtx cardinal.WorldContext) {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				_, _ = cardinal.GetComponent[UnregisteredComp](worldCtx, id)
			},
		},
		{
			name: "SetComponent",
			testFn: func(worldCtx cardinal.WorldContext) {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				_ = cardinal.SetComponent[UnregisteredComp](worldCtx, id, &UnregisteredComp{})
			},
		},
		{
			name: "UpdateComponent",
			testFn: func(worldCtx cardinal.WorldContext) {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				_ = cardinal.UpdateComponent[UnregisteredComp](worldCtx, id, func(u *UnregisteredComp) *UnregisteredComp {
					return u
				})
			},
		},
		{
			name: "Create",
			testFn: func(worldCtx cardinal.WorldContext) {
				_, _ = cardinal.Create(worldCtx, Foo{}, UnregisteredComp{})
			},
		},
		{
			name: "CreateMany",
			testFn: func(worldCtx cardinal.WorldContext) {
				_, _ = cardinal.CreateMany(worldCtx, 10, Foo{}, UnregisteredComp{})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			world, tick := testutils.MakeWorldAndTicker(t)
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			world.Init(func(worldCtx cardinal.WorldContext) error {
				defer func() {
					err := recover()
					assert.Check(t, err != nil, "expected the state mutation to panic")
					errStr, ok := err.(string)
					assert.Check(t, ok, "expected the panic to be of type string")
					isComponentNotRegistered := strings.Contains(errStr, cardinal.ErrComponentNotRegistered.Error())
					assert.Check(t, isComponentNotRegistered,
						fmt.Sprintf("expected error %q to contain %q",
							errStr,
							cardinal.ErrComponentNotRegistered.Error()))
				}()
				// This is expected to panic every time.
				tc.testFn(worldCtx)
				assert.Check(t, false, "should not reach this line; testFn should always panic")
				return nil
			})
			tick()
		})
	}
}

type QueryRequest struct{}
type QueryResponse struct{}

// TestReadContextsDoNotPanicOnComponentHasNotBeenRegistered ensures
func TestReadContextsDoNotPanicOnComponentHasNotBeenRegistered(t *testing.T) {
	// Read-only contexts should return "fatal" errors to the caller (i.e. it should not panic).
	testCases := []struct {
		name string
		// Every test is expected to panic, so no return error is needed
		testFn func(cardinal.WorldContext, cardinal.EntityID) error
	}{
		{
			name: "GetComponent",
			testFn: func(worldCtx cardinal.WorldContext, id cardinal.EntityID) error {
				_, err := cardinal.GetComponent[UnregisteredComp](worldCtx, id)
				return err
			},
		},
		{
			name: "SearchEach",
			testFn: func(worldCtx cardinal.WorldContext, id cardinal.EntityID) error {
				return worldCtx.NewSearch(cardinal.Exact(UnregisteredComp{})).
					Each(worldCtx, func(id cardinal.EntityID) bool {
						return true
					})
			},
		},
		{
			name: "SearchCount",
			testFn: func(worldCtx cardinal.WorldContext, id cardinal.EntityID) error {
				_, err := worldCtx.NewSearch(cardinal.Exact(UnregisteredComp{})).Count(worldCtx)
				return err
			},
		},
		{
			name: "SearchFirst",
			testFn: func(worldCtx cardinal.WorldContext, id cardinal.EntityID) error {
				_, err := worldCtx.NewSearch(cardinal.Exact(UnregisteredComp{})).First(worldCtx)
				return err
			},
		},
	}

	const queryName = "some_query"
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// In the happy path, this recover action isn't strictly necessary. These read-only contexts
			// shouldn't ever panic. This recover is included here so that if we DO panic, only the failing test
			// will be displayed in the failure logs.
			defer func() {
				err := recover()
				assert.Check(t, err == nil, "expected no panic but got %q", err)
			}()

			world, tick := testutils.MakeWorldAndTicker(t)
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			world.Init(func(worldCtx cardinal.WorldContext) error {
				_, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				return nil
			})
			err := cardinal.RegisterQuery[QueryRequest, QueryResponse](
				world,
				queryName,
				func(worldCtx cardinal.WorldContext, req *QueryRequest) (*QueryResponse, error) {
					id, err := worldCtx.NewSearch(cardinal.Exact(Foo{})).First(worldCtx)
					assert.Check(t, err == nil)
					return nil, tc.testFn(worldCtx, id)
				})
			assert.Check(t, err == nil)

			// Do an initial tick so that the single entity can be created.
			tick()

			query, err := world.Engine().GetQueryByName(queryName)
			assert.Check(t, err == nil)

			readOnlyWorldCtx := ecs.NewReadOnlyEngineContext(world.Engine())
			_, err = query.HandleQuery(readOnlyWorldCtx, QueryRequest{})
			// Each test case is meant to generate a "ErrComponentNotRegistered" error
			assert.Check(t, errors.Is(err, cardinal.ErrComponentNotRegistered),
				"expected a component not registered error, got %v", err)

		})
	}
}

func TestWriteContextsPanicOnRedisError(t *testing.T) {
	testCases := []struct {
		name   string
		failFn func(worldCtx cardinal.WorldContext, goodID cardinal.EntityID)
	}{
		{
			name: "AddComponentTo",
			failFn: func(worldCtx cardinal.WorldContext, goodID cardinal.EntityID) {
				_ = cardinal.AddComponentTo[Qux](worldCtx, goodID)
			},
		},
		{
			name: "RemoveComponentFrom",
			failFn: func(worldCtx cardinal.WorldContext, goodID cardinal.EntityID) {
				_ = cardinal.RemoveComponentFrom[Bar](worldCtx, goodID)
			},
		},
		{
			name: "GetComponent",
			failFn: func(worldCtx cardinal.WorldContext, goodID cardinal.EntityID) {
				_, _ = cardinal.GetComponent[Foo](worldCtx, goodID)
			},
		},
		{
			name: "SetComponent",
			failFn: func(worldCtx cardinal.WorldContext, goodID cardinal.EntityID) {
				_ = cardinal.SetComponent[Foo](worldCtx, goodID, &Foo{})
			},
		},
		{
			name: "UpdateComponent",
			failFn: func(worldCtx cardinal.WorldContext, goodID cardinal.EntityID) {
				_ = cardinal.UpdateComponent[Foo](worldCtx, goodID, func(f *Foo) *Foo {
					return f
				})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			miniRedis := miniredis.RunT(t)
			world, tick := testutils.MakeWorldAndTickerWithRedis(t, miniRedis)
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			assert.NilError(t, cardinal.RegisterComponent[Bar](world))
			assert.NilError(t, cardinal.RegisterComponent[Qux](world))

			// This system will be called 2 times. The first time, a single entity is created. The second time,
			// the previously created entity is fetched, and then miniRedis is closed. Subsequent attempts to access
			// data should panic.
			cardinal.RegisterSystems(world, func(worldCtx cardinal.WorldContext) error {
				// Set up the entity in the first tick
				if worldCtx.CurrentTick() == 0 {
					_, err := cardinal.Create(worldCtx, Foo{}, Bar{})
					assert.Check(t, err == nil)
					return nil
				}
				// Get the valid entity for the second tick
				id, err := worldCtx.NewSearch(cardinal.Exact(Foo{}, Bar{})).First(worldCtx)
				assert.Check(t, err == nil)
				assert.Check(t, id != storage.BadID)

				// Shut down redis. The testCase's failure function will now be able to fail
				miniRedis.Close()

				// Only set up this panic/recover expectation if we're in the second tick.
				defer func() {
					err := recover()
					assert.Check(t, err != nil, "expected panic")
				}()

				tc.failFn(worldCtx, id)
				assert.Check(t, false, "should never reach here")
				return nil
			})
			// The first tick sets up the entity
			tick()
			// The second tick calls the test case's failure function.
			tick()
		})
	}
}
func TestGetComponentInReadContextDoesNotPanicOnRedisError(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	world, tick := testutils.MakeWorldAndTickerWithRedis(t, miniRedis)
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))

	world.Init(func(worldCtx cardinal.WorldContext) error {
		_, err := cardinal.Create(worldCtx, Foo{})
		assert.Check(t, err == nil)
		return nil
	})

	const queryName = "some_query"
	assert.NilError(t, cardinal.RegisterQuery[QueryRequest, QueryResponse](
		world,
		queryName,
		func(worldCtx cardinal.WorldContext, req *QueryRequest) (*QueryResponse, error) {
			id, err := worldCtx.NewSearch(cardinal.Exact(Foo{})).First(worldCtx)
			assert.Check(t, err == nil)
			miniRedis.Close()

			_, err = cardinal.GetComponent[Foo](worldCtx, id)
			return nil, err
		}))

	// Tick so the entity can be created
	tick()

	query, err := world.Engine().GetQueryByName(queryName)
	assert.NilError(t, err)

	// Uhoh, redis is now broken.
	miniRedis.Close()

	readOnlyWorldCtx := ecs.NewReadOnlyEngineContext(world.Engine())
	// This will fail with a redis connection error, and since we're in a read-only context, we should NOT panic
	defer func() {
		assert.Check(t, recover() == nil, "expected no panic in a read-only context")
	}()

	_, err = query.HandleQuery(readOnlyWorldCtx, QueryRequest{})
	assert.ErrorContains(t, err, "connection refused", "expected a connection error")
}
