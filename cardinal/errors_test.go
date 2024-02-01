package cardinal_test

import (
	"errors"
	"fmt"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

// TestSystemsReturnNonFatalErrors ensures System will surface non-fatal read and write errors to the user.
func TestSystemsReturnNonFatalErrors(t *testing.T) {
	const nonExistentEntityID = 999
	testCases := []struct {
		name    string
		testFn  func(engine.Context) error
		wantErr error
	}{
		{
			name: "AddComponentTo_BadEntity",
			testFn: func(eCtx engine.Context) error {
				return cardinal.AddComponentTo[Foo](eCtx, nonExistentEntityID)
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "AddComponentTo_ComponentAlreadyOnEntity",
			testFn: func(eCtx engine.Context) error {
				id, err := cardinal.Create(eCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.AddComponentTo[Foo](eCtx, id)
			},
			wantErr: cardinal.ErrComponentAlreadyOnEntity,
		},
		{
			name: "RemoveComponentFrom_BadEntity",
			testFn: func(eCtx engine.Context) error {
				return cardinal.RemoveComponentFrom[Foo](eCtx, nonExistentEntityID)
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "RemoveComponentFrom_ComponentNotOnEntity",
			testFn: func(eCtx engine.Context) error {
				id, err := cardinal.Create(eCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.RemoveComponentFrom[Bar](eCtx, id)
			},
			wantErr: cardinal.ErrComponentNotOnEntity,
		},
		{
			name: "RemoveComponentFrom_EntityMustHaveAtLeastOneComponent",
			testFn: func(eCtx engine.Context) error {
				id, err := cardinal.Create(eCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.RemoveComponentFrom[Foo](eCtx, id)
			},
			wantErr: cardinal.ErrEntityMustHaveAtLeastOneComponent,
		},
		{
			name: "cardinal.GetComponent_BadEntity",
			testFn: func(eCtx engine.Context) error {
				_, err := cardinal.GetComponent[Foo](eCtx, nonExistentEntityID)
				return err
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "cardinal.GetComponent_ComponentNotOnEntity",
			testFn: func(eCtx engine.Context) error {
				id, err := cardinal.Create(eCtx, Foo{})
				assert.Check(t, err == nil)
				_, err = cardinal.GetComponent[Bar](eCtx, id)
				return err
			},
			wantErr: cardinal.ErrComponentNotOnEntity,
		},
		{
			name: "SetComponent_BadEntity",
			testFn: func(eCtx engine.Context) error {
				return cardinal.SetComponent[Foo](eCtx, nonExistentEntityID, &Foo{})
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "SetComponent_ComponentNotOnEntity",
			testFn: func(eCtx engine.Context) error {
				id, err := cardinal.Create(eCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.SetComponent[Bar](eCtx, id, &Bar{})
			},
			wantErr: cardinal.ErrComponentNotOnEntity,
		},
		{
			name: "UpdateComponent_BadEntity",
			testFn: func(eCtx engine.Context) error {
				return cardinal.UpdateComponent[Foo](eCtx, nonExistentEntityID, func(f *Foo) *Foo {
					return f
				})
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "UpdateComponent_ComponentNotOnEntity",
			testFn: func(eCtx engine.Context) error {
				id, err := cardinal.Create(eCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.UpdateComponent[Bar](eCtx, id, func(b *Bar) *Bar {
					return b
				})
			},
			wantErr: cardinal.ErrComponentNotOnEntity,
		},
		{
			name: "Remove_EntityDoesNotExist",
			testFn: func(eCtx engine.Context) error {
				return cardinal.Remove(eCtx, nonExistentEntityID)
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tf := testutils.NewTestFixture(t, nil)
			world, tick := tf.World, tf.DoTick
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			assert.NilError(t, cardinal.RegisterComponent[Bar](world))
			err := cardinal.RegisterInitSystems(world, func(eCtx engine.Context) error {
				defer func() {
					// In Systems, Cardinal is designed to panic when a fatal error is encountered.
					// This test is not supposed to panic, but if it does panic it happens in a non-main thread which
					// makes it hard to track down where the panic actually came from.
					// Recover here and complain about any non-nil panics to allow the remaining tests in this
					// function to be executed and so the maintainer will know exactly which test failed.
					err := recover()
					assert.Check(t, err == nil, "got fatal error \"%v\"", err)
				}()

				err := tc.testFn(eCtx)
				isWantError := errors.Is(err, tc.wantErr)
				assert.Check(t, isWantError, "expected %v but got %v", tc.wantErr, err)
				return nil
			})
			assert.NilError(t, err)
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
		panicFn func(engine.Context)
	}{
		{
			name: "cardinal.AddComponentTo",
			panicFn: func(eCtx engine.Context) {
				id, err := cardinal.Create(eCtx, Foo{})
				assert.Check(t, err == nil)
				_ = cardinal.AddComponentTo[UnregisteredComp](eCtx, id)
			},
		},
		{
			name: "cardinal.RemoveComponentFrom",
			panicFn: func(eCtx engine.Context) {
				id, err := cardinal.Create(eCtx, Foo{}, Bar{})
				assert.Check(t, err == nil)
				_ = cardinal.RemoveComponentFrom[UnregisteredComp](eCtx, id)
			},
		},
		{
			name: "cardinal.GetComponent",
			panicFn: func(eCtx engine.Context) {
				id, err := cardinal.Create(eCtx, Foo{})
				assert.Check(t, err == nil)
				_, _ = cardinal.GetComponent[UnregisteredComp](eCtx, id)
			},
		},
		{
			name: "cardinal.SetComponent",
			panicFn: func(eCtx engine.Context) {
				id, err := cardinal.Create(eCtx, Foo{})
				assert.Check(t, err == nil)
				_ = cardinal.SetComponent[UnregisteredComp](eCtx, id, &UnregisteredComp{})
			},
		},
		{
			name: "cardinal.UpdateComponent",
			panicFn: func(eCtx engine.Context) {
				id, err := cardinal.Create(eCtx, Foo{})
				assert.Check(t, err == nil)
				_ = cardinal.UpdateComponent[UnregisteredComp](eCtx, id,
					func(u *UnregisteredComp) *UnregisteredComp {
						return u
					})
			},
		},
		{
			name: "cardinal.Create",
			panicFn: func(eCtx engine.Context) {
				_, _ = cardinal.Create(eCtx, Foo{}, UnregisteredComp{})
			},
		},
		{
			name: "cardinal.CreateMany",
			panicFn: func(eCtx engine.Context) {
				_, _ = cardinal.CreateMany(eCtx, 10, Foo{}, UnregisteredComp{})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tf := testutils.NewTestFixture(t, nil)
			world, tick := tf.World, tf.DoTick
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			err := cardinal.RegisterInitSystems(world, func(eCtx engine.Context) error {
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
				tc.panicFn(eCtx)
				assert.Check(t, false, "should not reach this line")
				return nil
			})
			assert.NilError(t, err)
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
		testFn func(engine.Context) error
	}{
		{
			name: "cardinal.GetComponent",
			testFn: func(eCtx engine.Context) error {
				// Get a valid entity to ensure the error we find is related to the component and NOT
				// due to an invalid entity.
				id, err := cardinal.NewSearch(eCtx, filter.Exact(Foo{})).First()
				assert.Check(t, err == nil)
				_, err = cardinal.GetComponent[UnregisteredComp](eCtx, id)
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

			tf := testutils.NewTestFixture(t, nil)
			world, tick := tf.World, tf.DoTick
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			err := cardinal.RegisterInitSystems(world, func(eCtx engine.Context) error {
				// Make an entity so the test functions are operating on a valid entity.
				_, err := cardinal.Create(eCtx, Foo{})
				assert.Check(t, err == nil)
				return nil
			})
			assert.NilError(t, err)
			err = cardinal.RegisterQuery[QueryRequest, QueryResponse](
				world,
				queryName,
				func(eCtx engine.Context, req *QueryRequest) (*QueryResponse, error) {
					return nil, tc.testFn(eCtx)
				})
			assert.Check(t, err == nil)

			// Do an initial tick so that the single entity can be cardinal.Created.
			tick()

			query, err := world.GetQueryByName(queryName)
			assert.Check(t, err == nil)

			readOnlyEngineCtx := cardinal.NewReadOnlyWorldContext(world)
			_, err = query.HandleQuery(readOnlyEngineCtx, QueryRequest{})
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
		failFn func(eCtx engine.Context, goodID types.EntityID)
	}{
		{
			name: "cardinal.AddComponentTo",
			failFn: func(eCtx engine.Context, goodID types.EntityID) {
				_ = cardinal.AddComponentTo[Qux](eCtx, goodID)
			},
		},
		{
			name: "cardinal.RemoveComponentFrom",
			failFn: func(eCtx engine.Context, goodID types.EntityID) {
				_ = cardinal.RemoveComponentFrom[Bar](eCtx, goodID)
			},
		},
		{
			name: "cardinal.GetComponent",
			failFn: func(eCtx engine.Context, goodID types.EntityID) {
				_, _ = cardinal.GetComponent[Foo](eCtx, goodID)
			},
		},
		{
			name: "cardinal.SetComponent",
			failFn: func(eCtx engine.Context, goodID types.EntityID) {
				_ = cardinal.SetComponent[Foo](eCtx, goodID, &Foo{})
			},
		},
		{
			name: "cardinal.UpdateComponent",
			failFn: func(eCtx engine.Context, goodID types.EntityID) {
				_ = cardinal.UpdateComponent[Foo](eCtx, goodID, func(f *Foo) *Foo {
					return f
				})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			miniRedis := miniredis.RunT(t)
			tf := testutils.NewTestFixture(t, miniRedis)
			world, tick := tf.World, tf.DoTick
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			assert.NilError(t, cardinal.RegisterComponent[Bar](world))
			assert.NilError(t, cardinal.RegisterComponent[Qux](world))

			// This system will be called 2 times. The first time, a single entity is cardinal.Created. The second time,
			// the previously cardinal.Created entity is fetched, and then miniRedis is closed. Subsequent attempts to access
			// data should panic.
			assert.NilError(t, cardinal.RegisterSystems(world, func(eCtx engine.Context) error {
				// Set up the entity in the first tick
				if eCtx.CurrentTick() == 0 {
					_, err := cardinal.Create(eCtx, Foo{}, Bar{})
					assert.Check(t, err == nil)
					return nil
				}
				// Get the valid entity for the second tick
				id, err := cardinal.NewSearch(eCtx, filter.Exact(Foo{}, Bar{})).First()
				assert.Check(t, err == nil)
				assert.Check(t, id != iterators.BadID)

				// Shut down redis. The testCase's failure function will now be able to fail
				miniRedis.Close()

				// Only set up this panic/recover expectation if we're in the second tick.
				defer func() {
					err := recover()
					assert.Check(t, err != nil, "expected panic")
				}()

				tc.failFn(eCtx, id)
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
	tf := testutils.NewTestFixture(t, miniRedis)
	world, tick := tf.World, tf.DoTick
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))

	err := cardinal.RegisterSystems(world, func(eCtx engine.Context) error {
		_, err := cardinal.Create(eCtx, Foo{})
		assert.Check(t, err == nil)
		return nil
	})
	assert.NilError(t, err)

	const queryName = "some_query"
	assert.NilError(t, cardinal.RegisterQuery[QueryRequest, QueryResponse](
		world,
		queryName,
		func(eCtx engine.Context, req *QueryRequest) (*QueryResponse, error) {
			id, err := cardinal.NewSearch(eCtx, filter.Exact(Foo{})).First()
			assert.Check(t, err == nil)
			_, err = cardinal.GetComponent[Foo](eCtx, id)
			return nil, err
		}))

	// Tick so the entity can be cardinal.Created
	tick()

	query, err := world.GetQueryByName(queryName)
	assert.NilError(t, err)

	// Uhoh, redis is now broken.
	miniRedis.Close()

	readOnlyEngineCtx := cardinal.NewReadOnlyWorldContext(world)
	// This will fail with a redis connection error, and since we're in a Query, we should NOT panic
	defer func() {
		assert.Check(t, recover() == nil, "expected no panic in a query")
	}()

	_, err = query.HandleQuery(readOnlyEngineCtx, QueryRequest{})
	assert.ErrorContains(t, err, "connection refused", "expected a connection error")
}
