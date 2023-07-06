package base_shard

import (
	routerv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
	"context"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"gotest.tools/v3/assert"
	"reflect"
	"testing"
)

type FooTransaction struct {
	X uint64
}

type BarTransaction struct {
	Y uint64
}

// TestServer_SendMsg tests that when sending messages through to the EVM receiver server, they get passed along to
// the world, and executed in systems.
func TestServer_SendMsg(t *testing.T) {
	// setup the world
	w := inmem.NewECSWorldForTest(t)

	// build the dynamic ABI types for evm compat
	FooEvmTX, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "X", Type: "uint64"},
	})
	assert.NilError(t, err)
	FooEvmTX.TupleType = reflect.TypeOf(FooTransaction{})
	BarEvmTx, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "Y", Type: "uint64"},
	})
	assert.NilError(t, err)
	BarEvmTx.TupleType = reflect.TypeOf(BarTransaction{})

	// create the ECS transactions
	FooTx := ecs.NewTransactionType[FooTransaction]()
	BarTx := ecs.NewTransactionType[BarTransaction]()

	// bind them to EVM types
	FooTx.SetEVMType(FooEvmTX)
	BarTx.SetEVMType(BarEvmTx)
	assert.NilError(t, w.RegisterTransactions(FooTx, BarTx))

	// create some txs to submit
	fooTx1 := FooTransaction{X: 420}
	fooTxs := []FooTransaction{
		fooTx1,
	}
	barTx1 := BarTransaction{Y: 290}
	barTxs := []BarTransaction{
		barTx1,
	}

	// add a system that checks that they are submitted properly to the world.
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		inFooTxs := FooTx.In(queue)
		inBarTxs := BarTx.In(queue)
		assert.Equal(t, len(inFooTxs), len(fooTxs))
		assert.Equal(t, len(inBarTxs), len(barTxs))
		for i, tx := range inFooTxs {
			assert.DeepEqual(t, tx, fooTxs[i])
		}
		for i, tx := range inBarTxs {
			assert.DeepEqual(t, tx, barTxs[i])
		}
		return nil
	})
	assert.NilError(t, w.LoadGameState())

	// build the type map that is used by the EVM receiver server.
	txTypes := ITransactionTypes{
		FooTx.ID(): FooTx,
		BarTx.ID(): BarTx,
	}
	server := NewServer(txTypes, w)

	// marshal out the bytes to send from each list of transactions.
	for _, tx := range fooTxs {
		fooTxBz, err := abi.Arguments{{Type: FooEvmTX}}.Pack(tx)
		assert.NilError(t, err)
		_, err = server.SendMsg(context.Background(), &routerv1.MsgSend{
			Sender:    "hello",
			Message:   fooTxBz,
			MessageId: uint64(FooTx.ID()),
		})
		assert.NilError(t, err)
	}
	for _, tx := range barTxs {
		barTxBz, err := abi.Arguments{{Type: BarEvmTx}}.Pack(tx)
		assert.NilError(t, err)
		_, err = server.SendMsg(context.Background(), &routerv1.MsgSend{
			Sender:    "hello",
			Message:   barTxBz,
			MessageId: uint64(BarTx.ID()),
		})
		assert.NilError(t, err)
	}

	// we already sent the transactions through to the server, which should pipe them into the transaction queue.
	// all we need to do now is tick so the system can run, which will check that we got the transactions we just
	// piped through above.
	err = w.Tick()
	assert.NilError(t, err)

}
