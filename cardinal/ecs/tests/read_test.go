package tests

import (
	routerv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
	"context"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/evm"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"gotest.tools/v3/assert"
	"reflect"
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
	})
	// create the EVM request binding.
	FooEVMRequest, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "ID", Type: "string"},
	})
	assert.NilError(t, err)
	FooEVMRequest.TupleType = reflect.TypeOf(FooRequest{})

	// create the EVM reply binding.
	FooEVMReply, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "Name", Type: "string"},
		{Name: "Age", Type: "uint64"},
	})
	assert.NilError(t, err)
	FooEVMReply.TupleType = reflect.TypeOf(FooReply{})
	fooRead.SetEVMTypes(&FooEVMRequest, &FooEVMReply)
	w := inmem.NewECSWorldForTest(t)
	err = w.RegisterReads(fooRead)
	err = w.RegisterTransactions(ecs.NewTransactionType[struct{}]("blah"))
	assert.NilError(t, err)
	s, err := evm.NewServer(w)
	assert.NilError(t, err)

	// create the abi encoded bytes that the EVM would send.
	args := abi.Arguments{{Type: FooEVMRequest}}
	bz, err := args.Pack(FooRequest{ID: "foo"})
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
