package ecs_test

import (
	"context"
	"pkg.world.dev/world-engine/cardinal/shard/evm"
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
)

func TestQueryTypeNotStructs(t *testing.T) {
	str := "blah"
	err := ecs.RegisterQuery[string, string](
		testutils.NewTestFixture(t, nil).Engine,
		"foo",
		func(eCtx ecs.EngineContext, req *string) (*string, error) {
			return &str, nil
		},
	)
	assert.ErrorContains(t, err, "the Request and Reply generics must be both structs")
}

func TestQueryEVM(t *testing.T) {
	// --- TEST SETUP ---
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

	engine := testutils.NewTestFixture(t, nil).Engine
	err := ecs.RegisterQuery[FooRequest, FooReply](
		engine,
		"foo",
		func(eCtx ecs.EngineContext, req *FooRequest,
		) (*FooReply, error) {
			return &expectedReply, nil
		},
		ecs.WithQueryEVMSupport[FooRequest, FooReply](),
	)

	assert.NilError(t, err)
	err = engine.RegisterMessages(ecs.NewMessageType[struct{}, struct{}]("blah"))
	assert.NilError(t, err)
	s, err := evm.NewServer(engine)
	assert.NilError(t, err)

	// create the abi encoded bytes that the EVM would send.
	fooQuery, err := engine.GetQueryByName("foo")
	assert.NilError(t, err)
	bz, err := fooQuery.EncodeAsABI(FooRequest{ID: "foo"})
	assert.NilError(t, err)

	// query the resource.
	res, err := s.QueryShard(context.Background(), &routerv1.QueryShardRequest{
		Resource: fooQuery.Name(),
		Request:  bz,
	})
	assert.NilError(t, err)

	// decode the reply
	replyAny, err := fooQuery.DecodeEVMReply(res.Response)
	assert.NilError(t, err)

	// cast to reply type
	reply, ok := replyAny.(FooReply)
	assert.Equal(t, ok, true)
	// should be same!
	assert.Equal(t, reply, expectedReply)
}

func TestErrOnNoNameOrHandler(t *testing.T) {
	type foo struct{}
	testCases := []struct {
		name        string
		createQuery func() error
		shouldErr   bool
	}{
		{
			name: "error on no name",
			createQuery: func() error {
				return ecs.RegisterQuery[foo, foo](
					testutils.NewTestFixture(t, nil).Engine,
					"",
					nil)
			},
			shouldErr: true,
		},
		{
			name: "error on no handler",
			createQuery: func() error {
				return ecs.RegisterQuery[foo, foo](
					testutils.NewTestFixture(t, nil).Engine,
					"foo",
					nil)
			},
			shouldErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.shouldErr {
				err := tc.createQuery()
				assert.Assert(t, err != nil)
			} else {
				assert.NilError(t, tc.createQuery())
			}
		})
	}
}
