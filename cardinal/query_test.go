package cardinal_test

import (
	"errors"
	"testing"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"
)

type Health struct {
	Value int
}

func (Health) Name() string { return "health" }

type QueryHealthRequest struct {
	Min int
}

type QueryHealthResponse struct {
	IDs []types.EntityID
}

func handleQueryHealth(
	wCtx engine.Context,
	request *QueryHealthRequest,
) (*QueryHealthResponse, error) {
	resp := &QueryHealthResponse{}
	err := cardinal.NewSearch(wCtx, filter.Exact(Health{})).Each(func(id types.EntityID) bool {
		var err error
		var health *Health
		health, err = cardinal.GetComponent[Health](wCtx, id)
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
	_ = cardinal.RegisterQuery[FooReq, FooReply](
		testutils.NewTestFixture(t, nil).World,
		"query_health",
		func(
			_ engine.Context,
			_ *FooReq,
		) (*FooReply, error) {
			return &FooReply{}, errors.New("this function should never get called")
		},
		cardinal.WithQueryEVMSupport[FooReq, FooReply](),
	)
}

func TestQueryExample(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[Health](world))
	assert.NilError(
		t,
		cardinal.RegisterQuery[QueryHealthRequest, QueryHealthResponse](
			world,
			"query_health",
			handleQueryHealth,
		),
	)
	tf.StartWorld()
	worldCtx := cardinal.NewWorldContext(world)
	ids, err := cardinal.CreateMany(worldCtx, 100, Health{})
	assert.NilError(t, err)
	// Give each new entity health based on the ever-increasing index
	for i, id := range ids {
		assert.NilError(t, cardinal.UpdateComponent[Health](worldCtx, id, func(h *Health) *Health {
			h.Value = i
			return h
		}))
	}

	// No entities should have health over a million.
	q, err := world.GetQueryByName("query_health")
	assert.NilError(t, err)

	resp, err := q.HandleQuery(worldCtx, QueryHealthRequest{1_000_000})
	assert.NilError(t, err)
	assert.Equal(t, 0, len(resp.(*QueryHealthResponse).IDs))

	// All entities should have health over -100
	resp, err = q.HandleQuery(worldCtx, QueryHealthRequest{-100})
	assert.NilError(t, err)
	assert.Equal(t, 100, len(resp.(*QueryHealthResponse).IDs))

	// Exactly 10 entities should have health at or above 90
	resp, err = q.HandleQuery(worldCtx, QueryHealthRequest{90})
	assert.NilError(t, err)
	assert.Equal(t, 10, len(resp.(*QueryHealthResponse).IDs))
}

func TestQueryTypeNotStructs(t *testing.T) {
	str := "blah"
	err := cardinal.RegisterQuery[string, string](
		testutils.NewTestFixture(t, nil).World,
		"foo",
		func(wCtx engine.Context, req *string) (*string, error) {
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

	world := testutils.NewTestFixture(t, nil).World
	err := cardinal.RegisterQuery[FooRequest, FooReply](
		world,
		"foo",
		func(
			eCtx engine.Context, req *FooRequest,
		) (*FooReply, error) {
			return &expectedReply, nil
		},
		cardinal.WithQueryEVMSupport[FooRequest, FooReply](),
	)

	assert.NilError(t, err)
	err = cardinal.RegisterMessagesByName(world, message.NewMessageType[struct{}, struct{}]("blah"))
	assert.NilError(t, err)

	// create the abi encoded bytes that the EVM would send.
	fooQuery, err := world.GetQueryByName("foo")
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
				return cardinal.RegisterQuery[foo, foo](
					testutils.NewTestFixture(t, nil).World,
					"",
					nil)
			},
			shouldErr: true,
		},
		{
			name: "error on no handler",
			CreateQuery: func() error {
				return cardinal.RegisterQuery[foo, foo](
					testutils.NewTestFixture(t, nil).World,
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
