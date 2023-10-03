package ecs_test

import (
	"context"
	"testing"

	routerv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/evm"
	"pkg.world.dev/world-engine/cardinal/public"
)

func TestReadTypeNotStructs(t *testing.T) {
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
		ecs.NewReadType[FooRequest, FooReply]("foo", func(world public.IWorld, req FooRequest) (FooReply, error) {
			return expectedReply, nil
		})
		defer func() {
			//defered function should not fail
			panicValue := recover()
			assert.Assert(t, panicValue == nil)
		}()
	}()

	ecs.NewReadType[string, string]("foo", func(world public.IWorld, req string) (string, error) {
		return "blah", nil
	})
}

func TestReadEVM(t *testing.T) {
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
	fooRead := ecs.NewReadType[FooRequest, FooReply]("foo", func(world public.IWorld, req FooRequest) (FooReply, error) {
		return expectedReply, nil
	}, ecs.WithReadEVMSupport[FooRequest, FooReply])

	w := ecs.NewTestWorld(t)
	err := w.RegisterReads(fooRead)
	err = w.RegisterTransactions(ecs.NewTransactionType[struct{}, struct{}]("blah"))
	assert.NilError(t, err)
	s, err := evm.NewServer(w)
	assert.NilError(t, err)

	// create the abi encoded bytes that the EVM would send.
	bz, err := fooRead.EncodeAsABI(FooRequest{ID: "foo"})
	assert.NilError(t, err)

	// query the resource.
	res, err := s.QueryShard(context.Background(), &routerv1.QueryShardRequest{
		Resource: fooRead.Name(),
		Request:  bz,
	})
	assert.NilError(t, err)

	// decode the reply
	replyAny, err := fooRead.DecodeEVMReply(res.Response)
	assert.NilError(t, err)

	// cast to reply type
	reply, ok := replyAny.(FooReply)
	assert.Equal(t, ok, true)
	// should be same!
	assert.Equal(t, reply, expectedReply)
}
