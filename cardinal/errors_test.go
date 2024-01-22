package cardinal_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

// TestSystemsReturnNonFatalErrors ensures System will surface non-fatal read and write errors to the user.
func TestSystemsReturnNonFatalErrors(t *testing.T) {
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
			wantErr: cardinal.ErrEntityMustHaveAtLeastOneComponent,
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
			world, tick := testutils.MakeWorldAndTicker(t, nil)
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			assert.NilError(t, cardinal.RegisterComponent[Bar](world))
			world.Init(func(worldCtx cardinal.WorldContext) error {
				defer func() {
					// In Systems, Cardinal is designed to panic when a fatal error is encountered.
					// This test is not supposed to panic, but if it does panic it happens in a non-main thread which
					// makes it hard to track down where the panic actually came from.
					// Recover here and complain about any non-nil panics to allow the remaining tests in this
					// function to be executed and so the maintainer will know exactly which test failed.
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

// TestSystemPanicOnComponentHasNotBeenRegistered ensures Systems that encounter a component that has not been
// registered will panic.
func TestSystemsPanicOnComponentHasNotBeenRegistered(t *testing.T) {
	testCases := []struct {
		name string
		// Every test is expected to panic, so no return error is needed
		panicFn func(cardinal.WorldContext)
	}{
		{
			name: "AddComponentTo",
			panicFn: func(worldCtx cardinal.WorldContext) {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				_ = cardinal.AddComponentTo[UnregisteredComp](worldCtx, id)
			},
		},
		{
			name: "RemoveComponentFrom",
			panicFn: func(worldCtx cardinal.WorldContext) {
				id, err := cardinal.Create(worldCtx, Foo{}, Bar{})
				assert.Check(t, err == nil)
				_ = cardinal.RemoveComponentFrom[UnregisteredComp](worldCtx, id)
			},
		},
		{
			name: "GetComponent",
			panicFn: func(worldCtx cardinal.WorldContext) {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				_, _ = cardinal.GetComponent[UnregisteredComp](worldCtx, id)
			},
		},
		{
			name: "SetComponent",
			panicFn: func(worldCtx cardinal.WorldContext) {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				_ = cardinal.SetComponent[UnregisteredComp](worldCtx, id, &UnregisteredComp{})
			},
		},
		{
			name: "UpdateComponent",
			panicFn: func(worldCtx cardinal.WorldContext) {
				id, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				_ = cardinal.UpdateComponent[UnregisteredComp](worldCtx, id,
					func(u *UnregisteredComp) *UnregisteredComp {
						return u
					})
			},
		},
		{
			name: "Create",
			panicFn: func(worldCtx cardinal.WorldContext) {
				_, _ = cardinal.Create(worldCtx, Foo{}, UnregisteredComp{})
			},
		},
		{
			name: "CreateMany",
			panicFn: func(worldCtx cardinal.WorldContext) {
				_, _ = cardinal.CreateMany(worldCtx, 10, Foo{}, UnregisteredComp{})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			world, tick := testutils.MakeWorldAndTicker(t, nil)
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			world.Init(func(worldCtx cardinal.WorldContext) error {
				defer func() {
					err := recover()
					// assert.Check is required here because this is happening in a non-main thread.
					assert.Check(t, err != nil, "expected the state mutation to panic")
					errStr, ok := err.(string)
					assert.Check(t, ok, "expected the panic to be of type string")
					isErrComponentNotRegistered := strings.Contains(errStr, cardinal.ErrComponentNotRegistered.Error())
					assert.Check(t, isErrComponentNotRegistered,
						fmt.Sprintf("expected error %q to contain %q",
							errStr,
							cardinal.ErrComponentNotRegistered.Error()))
				}()
				// This should panic every time
				tc.panicFn(worldCtx)
				assert.Check(t, false, "should not reach this line")
				return nil
			})
			tick()
		})
	}
}

type QueryRequest struct{}
type QueryResponse struct{}

// TestQueriesDoNotPanicOnComponentHasNotBeenRegistered ensures queries do not panic when a non-registered component
// is encountered. Instead, the error should be returned to the user.
func TestQueriesDoNotPanicOnComponentHasNotBeenRegistered(t *testing.T) {
	testCases := []struct {
		name   string
		testFn func(cardinal.WorldContext) error
	}{
		{
			name: "GetComponent",
			testFn: func(worldCtx cardinal.WorldContext) error {
				// Get a valid entity to ensure the error we find is related to the component and NOT
				// due to an invalid entity.
				id, err := worldCtx.NewSearch(cardinal.Exact(Foo{})).First(worldCtx)
				assert.Check(t, err == nil)
				_, err = cardinal.GetComponent[UnregisteredComp](worldCtx, id)
				return err
			},
		},
	}

	const queryName = "some_query"
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// These queries shouldn't ever panic, but this recover is included here so that if we DO panic, only the
			// failing test will be displayed in the failure logs.
			defer func() {
				err := recover()
				assert.Check(t, err == nil, "expected no panic but got %q", err)
			}()

			world, tick := testutils.MakeWorldAndTicker(t, nil)
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			world.Init(func(worldCtx cardinal.WorldContext) error {
				// Make an entity so the test functions are operating on a valid entity.
				_, err := cardinal.Create(worldCtx, Foo{})
				assert.Check(t, err == nil)
				return nil
			})
			err := cardinal.RegisterQuery[QueryRequest, QueryResponse](
				world,
				queryName,
				func(worldCtx cardinal.WorldContext, req *QueryRequest) (*QueryResponse, error) {
					return nil, tc.testFn(worldCtx)
				})
			assert.Check(t, err == nil)

			// Do an initial tick so that the single entity can be created.
			tick()

			query, err := world.Engine().GetQueryByName(queryName)
			assert.Check(t, err == nil)

			readOnlyEnginwCtx := cardinal.NewReadOnlyWorldContext(world.Engine())
			_, err = query.HandleQuery(readOnlyEnginwCtx, QueryRequest{})
			// Each test case is meant to generate a "ErrComponentNotRegistered" error
			assert.Check(t, errors.Is(err, cardinal.ErrComponentNotRegistered),
				"expected a component not registered error, got %v", err)
		})
	}
}

// TestSystemsPanicOnRedisError ensures systems panic when there is a problem connecting to redis. In general, Systems
// should panic on ANY fatal error, but this connection problem is how we'll simulate a non ecs state related error.
func TestSystemsPanicOnRedisError(t *testing.T) {
	testCases := []struct {
		name string
		// the failFn will be called at a time when the ECB is empty of cached data and redis is down.
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
			world, tick := testutils.MakeWorldAndTicker(t, miniRedis)
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			assert.NilError(t, cardinal.RegisterComponent[Bar](world))
			assert.NilError(t, cardinal.RegisterComponent[Qux](world))

			// This system will be called 2 times. The first time, a single entity is created. The second time,
			// the previously created entity is fetched, and then miniRedis is closed. Subsequent attempts to access
			// data should panic.
			assert.NilError(t, cardinal.RegisterSystems(world, func(worldCtx cardinal.WorldContext) error {
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
			}))
			// The first tick sets up the entity
			tick()
			// The second tick calls the test case's failure function.
			tick()
		})
	}
}
func TestGetComponentInQueryDoesNotPanicOnRedisError(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	world, tick := testutils.MakeWorldAndTicker(t, miniRedis)
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
			_, err = cardinal.GetComponent[Foo](worldCtx, id)
			return nil, err
		}))

	// Tick so the entity can be created
	tick()

	query, err := world.Engine().GetQueryByName(queryName)
	assert.NilError(t, err)

	// Uhoh, redis is now broken.
	miniRedis.Close()

	readOnlyEnginwCtx := cardinal.NewReadOnlyWorldContext(world.Engine())
	// This will fail with a redis connection error, and since we're in a Query, we should NOT panic
	defer func() {
		assert.Check(t, recover() == nil, "expected no panic in a query")
	}()

	_, err = query.HandleQuery(readOnlyEnginwCtx, QueryRequest{})
	assert.ErrorContains(t, err, "connection refused", "expected a connection error")
}
