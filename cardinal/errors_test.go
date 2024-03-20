package cardinal_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/component"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

// TestSystemsReturnNonFatalErrors ensures System will surface non-fatal read and write errors to the user.
func TestSystemsReturnNonFatalErrors(t *testing.T) {
	const nonExistentEntityID = "-999"
	testCases := []struct {
		name    string
		testFn  func(engine.Context) error
		wantErr error
	}{
		{
			name: "AddComponentTo_BadEntity",
			testFn: func(wCtx engine.Context) error {
				return cardinal.AddComponentTo[Foo](wCtx, nonExistentEntityID)
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "AddComponentTo_ComponentAlreadyOnEntity",
			testFn: func(wCtx engine.Context) error {
				id, err := cardinal.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.AddComponentTo[Foo](wCtx, id)
			},
			wantErr: cardinal.ErrComponentAlreadyOnEntity,
		},
		{
			name: "RemoveComponentFrom_BadEntity",
			testFn: func(wCtx engine.Context) error {
				return cardinal.RemoveComponentFrom[Foo](wCtx, nonExistentEntityID)
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "RemoveComponentFrom_ComponentNotOnEntity",
			testFn: func(wCtx engine.Context) error {
				id, err := cardinal.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.RemoveComponentFrom[Bar](wCtx, id)
			},
			wantErr: cardinal.ErrComponentNotOnEntity,
		},
		{
			name: "RemoveComponentFrom_EntityMustHaveAtLeastOneComponent",
			testFn: func(wCtx engine.Context) error {
				id, err := cardinal.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.RemoveComponentFrom[Foo](wCtx, id)
			},
			wantErr: cardinal.ErrEntityMustHaveAtLeastOneComponent,
		},
		{
			name: "cardinal.GetComponent_BadEntity",
			testFn: func(wCtx engine.Context) error {
				_, err := cardinal.GetComponent[Foo](wCtx, nonExistentEntityID)
				return err
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "cardinal.GetComponent_ComponentNotOnEntity",
			testFn: func(wCtx engine.Context) error {
				id, err := cardinal.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_, err = cardinal.GetComponent[Bar](wCtx, id)
				return err
			},
			wantErr: cardinal.ErrComponentNotOnEntity,
		},
		{
			name: "SetComponent_BadEntity",
			testFn: func(wCtx engine.Context) error {
				return cardinal.SetComponent[Foo](wCtx, nonExistentEntityID, &Foo{})
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "SetComponent_ComponentNotOnEntity",
			testFn: func(wCtx engine.Context) error {
				id, err := cardinal.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.SetComponent[Bar](wCtx, id, &Bar{})
			},
			wantErr: cardinal.ErrComponentNotOnEntity,
		},
		{
			name: "UpdateComponent_BadEntity",
			testFn: func(wCtx engine.Context) error {
				return cardinal.UpdateComponent[Foo](wCtx, nonExistentEntityID, func(f *Foo) *Foo {
					return f
				})
			},
			wantErr: cardinal.ErrEntityDoesNotExist,
		},
		{
			name: "UpdateComponent_ComponentNotOnEntity",
			testFn: func(wCtx engine.Context) error {
				id, err := cardinal.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return cardinal.UpdateComponent[Bar](wCtx, id, func(b *Bar) *Bar {
					return b
				})
			},
			wantErr: cardinal.ErrComponentNotOnEntity,
		},
		{
			name: "Remove_EntityDoesNotExist",
			testFn: func(wCtx engine.Context) error {
				return cardinal.Remove(wCtx, nonExistentEntityID)
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
			err := cardinal.RegisterInitSystems(world, func(wCtx engine.Context) error {
				defer func() {
					// In Systems, Cardinal is designed to panic when a fatal error is encountered.
					// This test is not supposed to panic, but if it does panic it happens in a non-main thread which
					// makes it hard to track down where the panic actually came from.
					// Recover here and complain about any non-nil panics to allow the remaining tests in this
					// function to be executed and so the maintainer will know exactly which test failed.
					err := recover()
					assert.Check(t, err == nil, "got fatal error \"%v\"", err)
				}()

				err := tc.testFn(wCtx)
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
			panicFn: func(wCtx engine.Context) {
				id, err := cardinal.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_ = cardinal.AddComponentTo[UnregisteredComp](wCtx, id)
			},
		},
		{
			name: "cardinal.RemoveComponentFrom",
			panicFn: func(wCtx engine.Context) {
				id, err := cardinal.Create(wCtx, Foo{}, Bar{})
				assert.Check(t, err == nil)
				_ = cardinal.RemoveComponentFrom[UnregisteredComp](wCtx, id)
			},
		},
		{
			name: "cardinal.GetComponent",
			panicFn: func(wCtx engine.Context) {
				id, err := cardinal.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_, _ = cardinal.GetComponent[UnregisteredComp](wCtx, id)
			},
		},
		{
			name: "cardinal.SetComponent",
			panicFn: func(wCtx engine.Context) {
				id, err := cardinal.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_ = cardinal.SetComponent[UnregisteredComp](wCtx, id, &UnregisteredComp{})
			},
		},
		{
			name: "cardinal.UpdateComponent",
			panicFn: func(wCtx engine.Context) {
				id, err := cardinal.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_ = cardinal.UpdateComponent[UnregisteredComp](wCtx, id,
					func(u *UnregisteredComp) *UnregisteredComp {
						return u
					})
			},
		},
		{
			name: "cardinal.Create",
			panicFn: func(wCtx engine.Context) {
				_, _ = cardinal.Create(wCtx, Foo{}, UnregisteredComp{})
			},
		},
		{
			name: "cardinal.CreateMany",
			panicFn: func(wCtx engine.Context) {
				_, _ = cardinal.CreateMany(wCtx, 10, Foo{}, UnregisteredComp{})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tf := testutils.NewTestFixture(t, nil)
			world, tick := tf.World, tf.DoTick
			assert.NilError(t, cardinal.RegisterComponent[Foo](world))
			err := cardinal.RegisterInitSystems(world, func(wCtx engine.Context) error {
				defer func() {
					err := recover()
					// assert.Check is required here because this is happening in a non-main thread.
					assert.Check(t, err != nil, "expected the state mutation to panic")
					errStr, ok := err.(string)
					assert.Check(t, ok, "expected the panic to be of type string")
					isErrComponentNotRegistered := strings.Contains(errStr, component.ErrComponentNotRegistered.Error())
					assert.Check(t, isErrComponentNotRegistered,
						fmt.Sprintf("expected error %q to contain %q",
							errStr,
							component.ErrComponentNotRegistered.Error()))
				}()
				// This should panic every time
				tc.panicFn(wCtx)
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
			testFn: func(wCtx engine.Context) error {
				// Get a valid entity to ensure the error we find is related to the component and NOT
				// due to an invalid entity.
				id, err := cardinal.NewSearch(wCtx, filter.Exact(Foo{})).First()
				assert.Check(t, err == nil)
				_, err = cardinal.GetComponent[UnregisteredComp](wCtx, id)
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
			err := cardinal.RegisterInitSystems(world, func(wCtx engine.Context) error {
				// Make an entity so the test functions are operating on a valid entity.
				_, err := cardinal.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return nil
			})
			assert.NilError(t, err)
			err = cardinal.RegisterQuery[QueryRequest, QueryResponse](
				world,
				queryName,
				func(wCtx engine.Context, _ *QueryRequest) (*QueryResponse, error) {
					return nil, tc.testFn(wCtx)
				})
			assert.Check(t, err == nil)

			// Do an initial tick so that the single entity can be cardinal.Created.
			tick()

			query, err := world.GetQueryByName(queryName)
			assert.Check(t, err == nil)

			readOnlyWorldCtx := cardinal.NewReadOnlyWorldContext(world)
			_, err = query.HandleQuery(readOnlyWorldCtx, QueryRequest{})
			// Each test case is meant to generate a "ErrComponentNotRegistered" error
			assert.Check(t, errors.Is(err, component.ErrComponentNotRegistered),
				"expected a component not registered error, got %v", err)
		})
	}
}

func TestGetComponentInQueryDoesNotPanicOnRedisError(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	tf := testutils.NewTestFixture(t, miniRedis)
	world, tick := tf.World, tf.DoTick
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))

	err := cardinal.RegisterSystems(world, func(wCtx engine.Context) error {
		_, err := cardinal.Create(wCtx, Foo{})
		assert.Check(t, err == nil)
		return nil
	})
	assert.NilError(t, err)

	const queryName = "some_query"
	assert.NilError(t, cardinal.RegisterQuery[QueryRequest, QueryResponse](
		world,
		queryName,
		func(wCtx engine.Context, _ *QueryRequest) (*QueryResponse, error) {
			id, err := cardinal.NewSearch(wCtx, filter.Exact(Foo{})).First()
			assert.Check(t, err == nil)
			_, err = cardinal.GetComponent[Foo](wCtx, id)
			return nil, err
		}))

	// Tick so the entity can be cardinal.Created
	tick()

	query, err := world.GetQueryByName(queryName)
	assert.NilError(t, err)

	// Uhoh, redis is now broken.
	miniRedis.Close()

	readOnlyWorldCtx := cardinal.NewReadOnlyWorldContext(world)
	// This will fail with a redis connection error, and since we're in a Query, we should NOT panic
	defer func() {
		assert.Check(t, recover() == nil, "expected no panic in a query")
	}()

	_, err = query.HandleQuery(readOnlyWorldCtx, QueryRequest{})
	assert.ErrorContains(t, err, "connection refused", "expected a connection error")
}
