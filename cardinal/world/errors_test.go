package world_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/goccy/go-json"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/world"
)

type ScalarComponentStatic struct {
	Val int
}

func (ScalarComponentStatic) Name() string { return "scalar_component_static" }

type ScalarComponentDynamic struct {
	Val int
}

func (ScalarComponentDynamic) Name() string { return "scalar_component_dynamic" }

type Foo struct{}

func (Foo) Name() string { return "foo" }

type Bar struct{}

func (Bar) Name() string { return "bar" }

// TestSystemsReturnNonFatalErrors ensures system will surface non-fatal read and write errors to the user.
func TestSystemsReturnNonFatalErrors(t *testing.T) {
	const nonExistentEntityID = 999
	testCases := []struct {
		name    string
		testFn  func(world.WorldContext) error
		wantErr error
	}{
		{
			name: "AddComponentTo_BadEntity",
			testFn: func(wCtx world.WorldContext) error {
				return world.AddComponentTo[Foo](wCtx, nonExistentEntityID)
			},
			wantErr: gamestate.ErrEntityDoesNotExist,
		},
		{
			name: "AddComponentTo_ComponentAlreadyOnEntity",
			testFn: func(wCtx world.WorldContext) error {
				id, err := world.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return world.AddComponentTo[Foo](wCtx, id)
			},
			wantErr: gamestate.ErrComponentAlreadyOnEntity,
		},
		{
			name: "RemoveComponentFrom_BadEntity",
			testFn: func(wCtx world.WorldContext) error {
				return world.RemoveComponentFrom[Foo](wCtx, nonExistentEntityID)
			},
			wantErr: gamestate.ErrEntityDoesNotExist,
		},
		{
			name: "RemoveComponentFrom_ComponentNotOnEntity",
			testFn: func(wCtx world.WorldContext) error {
				id, err := world.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return world.RemoveComponentFrom[Bar](wCtx, id)
			},
			wantErr: gamestate.ErrComponentNotOnEntity,
		},
		{
			name: "RemoveComponentFrom_EntityMustHaveAtLeastOneComponent",
			testFn: func(wCtx world.WorldContext) error {
				id, err := world.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return world.RemoveComponentFrom[Foo](wCtx, id)
			},
			wantErr: gamestate.ErrEntityMustHaveAtLeastOneComponent,
		},
		{
			name: "GetComponent_BadEntity",
			testFn: func(wCtx world.WorldContext) error {
				_, err := world.GetComponent[Foo](wCtx, nonExistentEntityID)
				return err
			},
			wantErr: gamestate.ErrEntityDoesNotExist,
		},
		{
			name: "GetComponent_ComponentNotOnEntity",
			testFn: func(wCtx world.WorldContext) error {
				id, err := world.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_, err = world.GetComponent[Bar](wCtx, id)
				return err
			},
			wantErr: gamestate.ErrComponentNotOnEntity,
		},
		{
			name: "SetComponent_BadEntity",
			testFn: func(wCtx world.WorldContext) error {
				return world.SetComponent[Foo](wCtx, nonExistentEntityID, &Foo{})
			},
			wantErr: gamestate.ErrEntityDoesNotExist,
		},
		{
			name: "SetComponent_ComponentNotOnEntity",
			testFn: func(wCtx world.WorldContext) error {
				id, err := world.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return world.SetComponent[Bar](wCtx, id, &Bar{})
			},
			wantErr: gamestate.ErrComponentNotOnEntity,
		},
		{
			name: "UpdateComponent_BadEntity",
			testFn: func(wCtx world.WorldContext) error {
				return world.UpdateComponent[Foo](wCtx, nonExistentEntityID, func(f *Foo) *Foo {
					return f
				})
			},
			wantErr: gamestate.ErrEntityDoesNotExist,
		},
		{
			name: "UpdateComponent_ComponentNotOnEntity",
			testFn: func(wCtx world.WorldContext) error {
				id, err := world.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return world.UpdateComponent[Bar](wCtx, id, func(b *Bar) *Bar {
					return b
				})
			},
			wantErr: gamestate.ErrComponentNotOnEntity,
		},
		{
			name: "Remove_EntityDoesNotExist",
			testFn: func(wCtx world.WorldContext) error {
				return world.Remove(wCtx, nonExistentEntityID)
			},
			wantErr: gamestate.ErrEntityDoesNotExist,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tf := cardinal.NewTestCardinal(t, nil)
			tick := tf.DoTick
			assert.NilError(t, world.RegisterComponent[Foo](tf.World()))
			assert.NilError(t, world.RegisterComponent[Bar](tf.World()))
			err := world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
				defer func() {
					// In systems, Cardinal is designed to panic when a fatal error is encountered.
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
		panicFn func(world.WorldContext)
	}{
		{
			name: "AddComponentTo",
			panicFn: func(wCtx world.WorldContext) {
				id, err := world.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_ = world.AddComponentTo[UnregisteredComp](wCtx, id)
			},
		},
		{
			name: "RemoveComponentFrom",
			panicFn: func(wCtx world.WorldContext) {
				id, err := world.Create(wCtx, Foo{}, Bar{})
				assert.Check(t, err == nil)
				_ = world.RemoveComponentFrom[UnregisteredComp](wCtx, id)
			},
		},
		{
			name: "GetComponent",
			panicFn: func(wCtx world.WorldContext) {
				id, err := world.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_, _ = world.GetComponent[UnregisteredComp](wCtx, id)
			},
		},
		{
			name: "SetComponent",
			panicFn: func(wCtx world.WorldContext) {
				id, err := world.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_ = world.SetComponent[UnregisteredComp](wCtx, id, &UnregisteredComp{})
			},
		},
		{
			name: "UpdateComponent",
			panicFn: func(wCtx world.WorldContext) {
				id, err := world.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_ = world.UpdateComponent[UnregisteredComp](wCtx, id,
					func(u *UnregisteredComp) *UnregisteredComp {
						return u
					})
			},
		},
		{
			name: "Create",
			panicFn: func(wCtx world.WorldContext) {
				_, _ = world.Create(wCtx, Foo{}, UnregisteredComp{})
			},
		},
		{
			name: "CreateMany",
			panicFn: func(wCtx world.WorldContext) {
				_, _ = world.CreateMany(wCtx, 10, Foo{}, UnregisteredComp{})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tf := cardinal.NewTestCardinal(t, nil)
			tick := tf.DoTick
			assert.NilError(t, world.RegisterComponent[Foo](tf.World()))
			err := world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
				defer func() {
					err := recover()
					// assert.Check is required here because this is happening in a non-main thread.
					assert.Check(t, err != nil, "expected the ECB mutation to panic")
					errStr, ok := err.(string)
					assert.Check(t, ok, "expected the panic to be of type string")
					isErrComponentNotRegistered := strings.Contains(errStr, gamestate.ErrComponentNotRegistered.Error())
					assert.Check(t, isErrComponentNotRegistered,
						fmt.Sprintf("expected error %q to contain %q",
							errStr,
							gamestate.ErrComponentNotRegistered.Error()))
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
		testFn func(world.WorldContextReadOnly) error
	}{
		{
			name: "GetComponent",
			testFn: func(w world.WorldContextReadOnly) error {
				// Get a valid entity to ensure the error we find is related to the component and NOT
				// due to an invalid entity.
				id, err := w.Search(filter.Exact(Foo{})).First()
				assert.Check(t, err == nil)
				_, err = world.GetComponent[UnregisteredComp](w, id)
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

			tf := cardinal.NewTestCardinal(t, nil)
			tick := tf.DoTick
			assert.NilError(t, world.RegisterComponent[Foo](tf.World()))
			err := world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
				// Make an entity so the test functions are operating on a valid entity.
				_, err := world.Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return nil
			})
			assert.NilError(t, err)
			err = world.RegisterQuery[QueryRequest, QueryResponse](
				tf.World(),
				queryName,
				func(wCtx world.WorldContextReadOnly, _ *QueryRequest) (*QueryResponse, error) {
					return nil, tc.testFn(wCtx)
				})
			assert.Check(t, err == nil)

			// Do an initial tick so that the single entity can be Created.
			tick()

			reqBz, err := json.Marshal(QueryRequest{})
			assert.NilError(t, err)

			_, err = tf.Cardinal.World().HandleQuery("game", queryName, reqBz)
			// Each test case is meant to generate a "ErrComponentNotRegistered" error
			assert.Check(t, errors.Is(err, gamestate.ErrComponentNotRegistered),
				"expected a component not registered error, got %v", err)
		})
	}
}

func TestGetComponentInQueryDoesNotPanicOnRedisError(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	tick := tf.DoTick
	assert.NilError(t, world.RegisterComponent[Foo](tf.World()))

	err := world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
		_, err := world.Create(wCtx, Foo{})
		assert.Check(t, err == nil)
		return nil
	})
	assert.NilError(t, err)

	const queryName = "some_query"
	assert.NilError(t, world.RegisterQuery[QueryRequest, QueryResponse](
		tf.World(),
		queryName,
		func(wCtx world.WorldContextReadOnly, req *QueryRequest) (*QueryResponse, error) {
			id, err := wCtx.Search(filter.Exact(Foo{})).First()
			assert.Check(t, err != nil)
			_, err = world.GetComponent[Foo](wCtx, id)
			return nil, err
		}))

	// Tick so the entity can be Created
	tick()

	// Uhoh, redis is now broken.
	tf.Redis.Close()

	// This will fail with a redis connection error, and since we're in a query, we should NOT panic
	defer func() {
		assert.Check(t, recover() == nil, "expected no panic in a query")
	}()

	reqBz, err := json.Marshal(QueryRequest{})
	assert.NilError(t, err)

	_, err = tf.Cardinal.World().HandleQuery("game", queryName, reqBz)
	assert.IsError(t, err)
	assert.ErrorContains(t, err, "connection refused", "expected a connection error")
}
