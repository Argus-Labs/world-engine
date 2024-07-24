package cardinal

import (
	"errors"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

type Health struct {
	Value int
}

func (h Health) Name() string {
	return "health"
}

type QueryHealthRequest struct {
	Min int
}

type QueryHealthResponse struct {
	IDs []types.EntityID
}

func handleQueryHealth(
	wCtx WorldContext,
	request *QueryHealthRequest,
) (*QueryHealthResponse, error) {
	resp := &QueryHealthResponse{}
	err := NewSearch().Entity(filter.Exact(filter.Component[Health]())).Each(wCtx, func(id types.EntityID) bool {
		var err error
		var health *Health
		health, err = GetComponent[Health](wCtx, id)
		if err != nil {
			return true
		}
		if health.Value < request.Min {
			return true
		}
		resp.IDs = append(resp.IDs, id)
		return true
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func TestNewQueryTypeWithEVMSupport(t *testing.T) {
	// This test just makes sure that NeQueryTypeWithEVMSupport maintains api compatibility.
	// it is mainly here to check for compiler errors.
	type FooReq struct {
		X uint64
	}
	type FooReply struct {
		Y uint64
	}
	_ = RegisterQuery[FooReq, FooReply](
		NewTestFixture(t, nil).World,
		"query_health",
		func(
			_ WorldContext,
			_ *FooReq,
		) (*FooReply, error) {
			return &FooReply{}, errors.New("this function should never get called")
		},
		WithQueryEVMSupport[FooReq, FooReply](),
	)
}

func TestQueryExample(t *testing.T) {
	tf := NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, RegisterComponent[Health](world))
	assert.NilError(
		t,
		RegisterQuery[QueryHealthRequest, QueryHealthResponse](
			world,
			"query_health",
			handleQueryHealth,
		),
	)
	tf.StartWorld()
	worldCtx := NewWorldContext(world)
	ids, err := CreateMany(worldCtx, 100, Health{})
	assert.NilError(t, err)
	// Give each new entity health based on the ever-increasing index
	for i, id := range ids {
		assert.NilError(t, UpdateComponent[Health](worldCtx, id, func(h *Health) *Health {
			h.Value = i
			return h
		}))
	}

	// No entities should have health over a million.
	q, err := world.GetQuery(DefaultQueryGroup, "query_health")
	assert.NilError(t, err)

	resp, err := q.handleQuery(worldCtx, QueryHealthRequest{1_000_000})
	assert.NilError(t, err)
	assert.Equal(t, 0, len(resp.(*QueryHealthResponse).IDs))

	// All entities should have health over -100
	resp, err = q.handleQuery(worldCtx, QueryHealthRequest{-100})
	assert.NilError(t, err)
	assert.Equal(t, 100, len(resp.(*QueryHealthResponse).IDs))

	// Exactly 10 entities should have health at or above 90
	resp, err = q.handleQuery(worldCtx, QueryHealthRequest{90})
	assert.NilError(t, err)
	assert.Equal(t, 10, len(resp.(*QueryHealthResponse).IDs))
}

func TestQueryTypeNotStructs(t *testing.T) {
	str := "blah"
	err := RegisterQuery[string, string](
		NewTestFixture(t, nil).World,
		"foo",
		func(WorldContext, *string) (*string, error) {
			return &str, nil
		},
	)
	assert.ErrorContains(t, err, "the Request and Reply generics must be both structs")
}

func TestQueryEVM(t *testing.T) {
	type FooRequest struct {
		ID string
	}
	type FooReply struct {
		Name string
		Age  uint64
	}

	expectedReply := FooReply{
		Name: "Chad",
		Age:  22,
	}

	world := NewTestFixture(t, nil).World
	err := RegisterQuery[FooRequest, FooReply](
		world,
		"foo",
		func(
			_ WorldContext, _ *FooRequest,
		) (*FooReply, error) {
			return &expectedReply, nil
		},
		WithQueryEVMSupport[FooRequest, FooReply](),
	)

	assert.NilError(t, err)
	err = RegisterMessage[struct{}, struct{}](world, "blah")
	assert.NilError(t, err)

	// create the abi encoded bytes that the EVM would send.
	fooQuery, err := world.GetQuery(DefaultQueryGroup, "foo")
	assert.NilError(t, err)
	bz, err := fooQuery.EncodeAsABI(FooRequest{ID: "foo"})
	assert.NilError(t, err)

	// query the resource.
	bz, err = world.HandleEVMQuery("foo", bz)
	assert.NilError(t, err)

	reply, err := fooQuery.DecodeEVMReply(bz)
	assert.NilError(t, err)

	gotReply, ok := reply.(FooReply)
	assert.True(t, ok, "could not cast %T to %T", reply, FooReply{})

	assert.Equal(t, gotReply, expectedReply)
}

func TestErrOnNoNameOrHandler(t *testing.T) {
	type foo struct{}
	testCases := []struct {
		name        string
		CreateQuery func() error
		shouldErr   bool
	}{
		{
			name: "error on no name",
			CreateQuery: func() error {
				return RegisterQuery[foo, foo](
					NewTestFixture(t, nil).World,
					"",
					nil)
			},
			shouldErr: true,
		},
		{
			name: "error on no handler",
			CreateQuery: func() error {
				return RegisterQuery[foo, foo](
					NewTestFixture(t, nil).World,
					"foo",
					nil)
			},
			shouldErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.shouldErr {
				err := tc.CreateQuery()
				assert.Assert(t, err != nil)
			} else {
				assert.NilError(t, tc.CreateQuery())
			}
		})
	}
}
