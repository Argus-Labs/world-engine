package ecs_test

import (
	"context"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"testing"

	"pkg.world.dev/world-engine/cardinal/evm"

	"gotest.tools/v3/assert"

	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
)

func TestQueryTypeNotStructs(t *testing.T) {
	str := "blah"
	err := ecs.RegisterQuery[string, string](testutils.NewTestWorld(t).Instance(), "foo", func(wCtx ecs.WorldContext, req *string) (*string, error) {
		return &str, nil
	})
	assert.Assert(t, err != nil)
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

	w := testutils.NewTestWorld(t).Instance()
	err := ecs.RegisterQuery[FooRequest, FooReply](w, "foo", func(wCtx ecs.WorldContext, req *FooRequest,
	) (*FooReply, error) {
		return &expectedReply, nil
	}, ecs.WithQueryEVMSupport[FooRequest, FooReply])

	assert.NilError(t, err)
	err = w.RegisterMessages(ecs.NewMessageType[struct{}, struct{}]("blah"))
	assert.NilError(t, err)
	s, err := evm.NewServer(w)
	assert.NilError(t, err)

	// create the abi encoded bytes that the EVM would send.
	fooQuery, err := w.GetQueryByName("foo")
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

func TestPanicsOnNoNameOrHandler(t *testing.T) {
	type foo struct{}
	testCases := []struct {
		name        string
		createQuery func() error
		shouldErr   bool
	}{
		{
			name: "panic on no name",
			createQuery: func() error {
				return ecs.RegisterQuery[foo, foo](testutils.NewTestWorld(t).Instance(), "", nil)
			},
			shouldErr: true,
		},
		{
			name: "panic on no handler",
			createQuery: func() error {
				return ecs.RegisterQuery[foo, foo](testutils.NewTestWorld(t).Instance(), "foo", nil)
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
