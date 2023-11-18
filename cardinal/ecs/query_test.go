package ecs_test

import (
	"context"
	"testing"

	"pkg.world.dev/world-engine/cardinal/cardinaltestutils"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/cardinal/evm"

	"gotest.tools/v3/assert"

	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
)

func TestQueryTypeNotStructs(t *testing.T) {
	str := "blah"
	err := ecs.RegisterQuery[string, string](
		cardinaltestutils.NewTestWorld(t).Instance(),
		"foo",
		func(wCtx ecs.WorldContext, req *string) (*string, error) {
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

	w := cardinaltestutils.NewTestWorld(t).Instance()
	err := ecs.RegisterQuery[FooRequest, FooReply](
		w,
		"foo",
		func(wCtx ecs.WorldContext, req *FooRequest,
		) (*FooReply, error) {
			return &expectedReply, nil
		},
		ecs.WithQueryEVMSupport[FooRequest, FooReply],
	)

	testutils.AssertNilErrorWithTrace(t, err)
	err = w.RegisterMessages(ecs.NewMessageType[struct{}, struct{}]("blah"))
	testutils.AssertNilErrorWithTrace(t, err)
	s, err := evm.NewServer(w)
	testutils.AssertNilErrorWithTrace(t, err)

	// create the abi encoded bytes that the EVM would send.
	fooQuery, err := w.GetQueryByName("foo")
	testutils.AssertNilErrorWithTrace(t, err)
	bz, err := fooQuery.EncodeAsABI(FooRequest{ID: "foo"})
	testutils.AssertNilErrorWithTrace(t, err)

	// query the resource.
	res, err := s.QueryShard(context.Background(), &routerv1.QueryShardRequest{
		Resource: fooQuery.Name(),
		Request:  bz,
	})
	testutils.AssertNilErrorWithTrace(t, err)

	// decode the reply
	replyAny, err := fooQuery.DecodeEVMReply(res.Response)
	testutils.AssertNilErrorWithTrace(t, err)

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
				return ecs.RegisterQuery[foo, foo](cardinaltestutils.NewTestWorld(t).Instance(), "", nil)
			},
			shouldErr: true,
		},
		{
			name: "error on no handler",
			createQuery: func() error {
				return ecs.RegisterQuery[foo, foo](cardinaltestutils.NewTestWorld(t).Instance(), "foo", nil)
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
				testutils.AssertNilErrorWithTrace(t, tc.createQuery())
			}
		})
	}
}
