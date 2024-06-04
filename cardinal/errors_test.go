package cardinal

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/component"
	"pkg.world.dev/world-engine/cardinal/search/filter"
)

// TestSystemsReturnNonFatalErrors ensures system will surface non-fatal read and write errors to the user.
func TestSystemsReturnNonFatalErrors(t *testing.T) {
	const nonExistentEntityID = 999
	testCases := []struct {
		name    string
		testFn  func(WorldContext) error
		wantErr error
	}{
		{
			name: "AddComponentTo_BadEntity",
			testFn: func(wCtx WorldContext) error {
				return AddComponentTo[Foo](wCtx, nonExistentEntityID)
			},
			wantErr: ErrEntityDoesNotExist,
		},
		{
			name: "AddComponentTo_ComponentAlreadyOnEntity",
			testFn: func(wCtx WorldContext) error {
				id, err := Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return AddComponentTo[Foo](wCtx, id)
			},
			wantErr: ErrComponentAlreadyOnEntity,
		},
		{
			name: "RemoveComponentFrom_BadEntity",
			testFn: func(wCtx WorldContext) error {
				return RemoveComponentFrom[Foo](wCtx, nonExistentEntityID)
			},
			wantErr: ErrEntityDoesNotExist,
		},
		{
			name: "RemoveComponentFrom_ComponentNotOnEntity",
			testFn: func(wCtx WorldContext) error {
				id, err := Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return RemoveComponentFrom[Bar](wCtx, id)
			},
			wantErr: ErrComponentNotOnEntity,
		},
		{
			name: "RemoveComponentFrom_EntityMustHaveAtLeastOneComponent",
			testFn: func(wCtx WorldContext) error {
				id, err := Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return RemoveComponentFrom[Foo](wCtx, id)
			},
			wantErr: ErrEntityMustHaveAtLeastOneComponent,
		},
		{
			name: "GetComponent_BadEntity",
			testFn: func(wCtx WorldContext) error {
				_, err := GetComponent[Foo](wCtx, nonExistentEntityID)
				return err
			},
			wantErr: ErrEntityDoesNotExist,
		},
		{
			name: "GetComponent_ComponentNotOnEntity",
			testFn: func(wCtx WorldContext) error {
				id, err := Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_, err = GetComponent[Bar](wCtx, id)
				return err
			},
			wantErr: ErrComponentNotOnEntity,
		},
		{
			name: "SetComponent_BadEntity",
			testFn: func(wCtx WorldContext) error {
				return SetComponent[Foo](wCtx, nonExistentEntityID, &Foo{})
			},
			wantErr: ErrEntityDoesNotExist,
		},
		{
			name: "SetComponent_ComponentNotOnEntity",
			testFn: func(wCtx WorldContext) error {
				id, err := Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return SetComponent[Bar](wCtx, id, &Bar{})
			},
			wantErr: ErrComponentNotOnEntity,
		},
		{
			name: "UpdateComponent_BadEntity",
			testFn: func(wCtx WorldContext) error {
				return UpdateComponent[Foo](wCtx, nonExistentEntityID, func(f *Foo) *Foo {
					return f
				})
			},
			wantErr: ErrEntityDoesNotExist,
		},
		{
			name: "UpdateComponent_ComponentNotOnEntity",
			testFn: func(wCtx WorldContext) error {
				id, err := Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return UpdateComponent[Bar](wCtx, id, func(b *Bar) *Bar {
					return b
				})
			},
			wantErr: ErrComponentNotOnEntity,
		},
		{
			name: "Remove_EntityDoesNotExist",
			testFn: func(wCtx WorldContext) error {
				return Remove(wCtx, nonExistentEntityID)
			},
			wantErr: ErrEntityDoesNotExist,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tf := NewTestFixture(t, nil)
			world, tick := tf.World, tf.DoTick
			assert.NilError(t, RegisterComponent[Foo](world))
			assert.NilError(t, RegisterComponent[Bar](world))
			err := RegisterInitSystems(world, func(wCtx WorldContext) error {
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
		panicFn func(WorldContext)
	}{
		{
			name: "AddComponentTo",
			panicFn: func(wCtx WorldContext) {
				id, err := Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_ = AddComponentTo[UnregisteredComp](wCtx, id)
			},
		},
		{
			name: "RemoveComponentFrom",
			panicFn: func(wCtx WorldContext) {
				id, err := Create(wCtx, Foo{}, Bar{})
				assert.Check(t, err == nil)
				_ = RemoveComponentFrom[UnregisteredComp](wCtx, id)
			},
		},
		{
			name: "GetComponent",
			panicFn: func(wCtx WorldContext) {
				id, err := Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_, _ = GetComponent[UnregisteredComp](wCtx, id)
			},
		},
		{
			name: "SetComponent",
			panicFn: func(wCtx WorldContext) {
				id, err := Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_ = SetComponent[UnregisteredComp](wCtx, id, &UnregisteredComp{})
			},
		},
		{
			name: "UpdateComponent",
			panicFn: func(wCtx WorldContext) {
				id, err := Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				_ = UpdateComponent[UnregisteredComp](wCtx, id,
					func(u *UnregisteredComp) *UnregisteredComp {
						return u
					})
			},
		},
		{
			name: "Create",
			panicFn: func(wCtx WorldContext) {
				_, _ = Create(wCtx, Foo{}, UnregisteredComp{})
			},
		},
		{
			name: "CreateMany",
			panicFn: func(wCtx WorldContext) {
				_, _ = CreateMany(wCtx, 10, Foo{}, UnregisteredComp{})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tf := NewTestFixture(t, nil)
			world, tick := tf.World, tf.DoTick
			assert.NilError(t, RegisterComponent[Foo](world))
			err := RegisterInitSystems(world, func(wCtx WorldContext) error {
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
		testFn func(WorldContext) error
	}{
		{
			name: "GetComponent",
			testFn: func(wCtx WorldContext) error {
				// Get a valid entity to ensure the error we find is related to the component and NOT
				// due to an invalid entity.
				id, err := NewSearch().Entity(filter.Exact(filter.Component[Foo]())).First(wCtx)
				assert.Check(t, err == nil)
				_, err = GetComponent[UnregisteredComp](wCtx, id)
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

			tf := NewTestFixture(t, nil)
			world, tick := tf.World, tf.DoTick
			assert.NilError(t, RegisterComponent[Foo](world))
			err := RegisterInitSystems(world, func(wCtx WorldContext) error {
				// Make an entity so the test functions are operating on a valid entity.
				_, err := Create(wCtx, Foo{})
				assert.Check(t, err == nil)
				return nil
			})
			assert.NilError(t, err)
			err = RegisterQuery[QueryRequest, QueryResponse](
				world,
				queryName,
				func(wCtx WorldContext, _ *QueryRequest) (*QueryResponse, error) {
					return nil, tc.testFn(wCtx)
				})
			assert.Check(t, err == nil)

			// Do an initial tick so that the single entity can be Created.
			tick()

			query, err := world.GetQuery(DefaultQueryGroup, queryName)
			assert.Check(t, err == nil)

			readOnlyWorldCtx := NewReadOnlyWorldContext(world)
			_, err = query.handleQuery(readOnlyWorldCtx, QueryRequest{})
			// Each test case is meant to generate a "ErrComponentNotRegistered" error
			assert.Check(t, errors.Is(err, component.ErrComponentNotRegistered),
				"expected a component not registered error, got %v", err)
		})
	}
}

func TestGetComponentInQueryDoesNotPanicOnRedisError(t *testing.T) {
	tf := NewTestFixture(t, nil)
	world, tick := tf.World, tf.DoTick
	assert.NilError(t, RegisterComponent[Foo](world))

	err := RegisterSystems(world, func(wCtx WorldContext) error {
		_, err := Create(wCtx, Foo{})
		assert.Check(t, err == nil)
		return nil
	})
	assert.NilError(t, err)

	const queryName = "some_query"
	assert.NilError(t, RegisterQuery[QueryRequest, QueryResponse](
		world,
		queryName,
		func(wCtx WorldContext, _ *QueryRequest) (*QueryResponse, error) {
			id, err := NewSearch().Entity(filter.Exact(filter.Component[Foo]())).First(wCtx)
			assert.Check(t, err == nil)
			_, err = GetComponent[Foo](wCtx, id)
			return nil, err
		}))

	// Tick so the entity can be Created
	tick()

	query, err := world.GetQuery(DefaultQueryGroup, queryName)
	assert.NilError(t, err)

	// Uhoh, redis is now broken.
	tf.Redis.Close()

	readOnlyWorldCtx := NewReadOnlyWorldContext(world)
	// This will fail with a redis connection error, and since we're in a query, we should NOT panic
	defer func() {
		assert.Check(t, recover() == nil, "expected no panic in a query")
	}()

	_, err = query.handleQuery(readOnlyWorldCtx, QueryRequest{})
	assert.ErrorContains(t, err, "connection refused", "expected a connection error")
}
