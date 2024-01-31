package ecs_test

import (
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"
)

func TestQueryTypeNotStructs(t *testing.T) {
	str := "blah"
	err := cardinal.RegisterQuery[string, string](
		testutils.NewTestFixture(t, nil).World,
		"foo",
		func(eCtx engine.Context, req *string) (*string, error) {
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

	eng := testutils.NewTestFixture(t, nil).Engine
	err := ecs.RegisterQuery[FooRequest, FooReply](
		eng,
		"foo",
		func(
			eCtx engine.Context, req *FooRequest,
		) (*FooReply, error) {
			return &expectedReply, nil
		},
		ecs.WithQueryEVMSupport[FooRequest, FooReply](),
	)

	assert.NilError(t, err)
	err = eng.RegisterMessages(ecs.NewMessageType[struct{}, struct{}]("blah"))
	assert.NilError(t, err)

	// create the abi encoded bytes that the EVM would send.
	fooQuery, err := eng.GetQueryByName("foo")
	assert.NilError(t, err)
	bz, err := fooQuery.EncodeAsABI(FooRequest{ID: "foo"})
	assert.NilError(t, err)

	// query the resource.
	bz, err = engine.HandleEVMQuery("foo", bz)
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
		createQuery func() error
		shouldErr   bool
	}{
		{
			name: "error on no name",
			createQuery: func() error {
				return cardinal.RegisterQuery[foo, foo](
					testutils.NewTestFixture(t, nil).World,
					"",
					nil)
			},
			shouldErr: true,
		},
		{
			name: "error on no handler",
			createQuery: func() error {
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
				err := tc.createQuery()
				assert.Assert(t, err != nil)
			} else {
				assert.NilError(t, tc.createQuery())
			}
		})
	}
}
