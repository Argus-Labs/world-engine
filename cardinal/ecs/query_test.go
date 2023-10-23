package ecs_test

import (
	"context"
	"testing"

	"pkg.world.dev/world-engine/cardinal/evm"

	"gotest.tools/v3/assert"

	routerv1 "pkg.world.dev/world-engine/rift/router/v1"

	"pkg.world.dev/world-engine/cardinal/ecs"
)

func TestQueryTypeNotStructs(t *testing.T) {
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

	defer func() {
		// test should trigger a panic.
		panicValue := recover()
		assert.Assert(t, panicValue != nil)
		ecs.NewQueryType[FooRequest, FooReply]("foo", func(wCtx ecs.WorldContext, req FooRequest) (FooReply, error) {
			return expectedReply, nil
		})
		defer func() {
			//defered function should not fail
			panicValue := recover()
			assert.Assert(t, panicValue == nil)
		}()
	}()

	ecs.NewQueryType[string, string]("foo", func(wCtx ecs.WorldContext, req string) (string, error) {
		return "blah", nil
	})
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
	fooQuery := ecs.NewQueryType[FooRequest, FooReply]("foo", func(wCtx ecs.WorldContext, req FooRequest) (FooReply, error) {
		return expectedReply, nil
	}, ecs.WithQueryEVMSupport[FooRequest, FooReply])

	w := ecs.NewTestWorld(t)
	err := w.RegisterQueries(fooQuery)
	err = w.RegisterTransactions(ecs.NewTransactionType[struct{}, struct{}]("blah"))
	assert.NilError(t, err)
	s, err := evm.NewServer(w)
	assert.NilError(t, err)

	// create the abi encoded bytes that the EVM would send.
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
