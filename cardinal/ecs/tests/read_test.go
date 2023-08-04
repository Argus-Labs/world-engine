package tests

import (
	routerv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
	"context"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/evm"
	"gotest.tools/v3/assert"
	"testing"
)

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
	fooRead := ecs.NewReadType[FooRequest, FooReply]("foo", func(world *ecs.World, req FooRequest) (FooReply, error) {
		return expectedReply, nil
	}, ecs.WithEVMSupport[FooRequest, FooReply])

	w := inmem.NewECSWorldForTest(t)
	err := w.RegisterReads(fooRead)
	err = w.RegisterTransactions(ecs.NewTransactionType[struct{}]("blah"))
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
