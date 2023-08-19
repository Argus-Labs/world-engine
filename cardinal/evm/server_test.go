package evm

import (
	"context"
	"github.com/rs/zerolog"
	"testing"

	routerv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
)

type FooTransaction struct {
	X uint64
	Y string
}

type BarTransaction struct {
	Y uint64
	Z bool
}

type TxReply struct{}

// TestServer_SendMessage tests that when sending messages through to the EVM receiver server, they get passed along to
// the world, and executed in systems.
func TestServer_SendMessage(t *testing.T) {
	// setup the world
	w := inmem.NewECSWorldForTest(t)

	// create the ECS transactions
	FooTx := ecs.NewTransactionType[FooTransaction, TxReply]("footx", ecs.WithTxEVMSupport[FooTransaction, TxReply])
	BarTx := ecs.NewTransactionType[BarTransaction, TxReply]("bartx", ecs.WithTxEVMSupport[BarTransaction, TxReply])

	assert.NilError(t, w.RegisterTransactions(FooTx, BarTx))

	// create some txs to submit

	fooTxs := []FooTransaction{
		{X: 420, Y: "world"},
		{X: 3290, Y: "earth"},
		{X: 411, Y: "universe"},
	}
	barTxs := []BarTransaction{
		{Y: 290, Z: true},
		{Y: 400, Z: false},
	}

	// add a system that checks that they are submitted properly to the world.
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue, _ *zerolog.Logger) error {
		inFooTxs := FooTx.In(queue)
		inBarTxs := BarTx.In(queue)
		assert.Equal(t, len(inFooTxs), len(fooTxs))
		assert.Equal(t, len(inBarTxs), len(barTxs))
		for i, tx := range inFooTxs {
			assert.DeepEqual(t, tx.Value, fooTxs[i])
		}
		for i, tx := range inBarTxs {
			assert.DeepEqual(t, tx.Value, barTxs[i])
		}
		return nil
	})
	assert.NilError(t, w.LoadGameState())

	server, err := NewServer(w)
	assert.NilError(t, err)

	// marshal out the bytes to send from each list of transactions.
	for _, tx := range fooTxs {
		fooTxBz, err := FooTx.ABIEncode(tx)
		assert.NilError(t, err)
		_, err = server.SendMessage(context.Background(), &routerv1.SendMessageRequest{
			Sender:    "hello",
			Message:   fooTxBz,
			MessageId: uint64(FooTx.ID()),
		})
		assert.NilError(t, err)
	}
	for _, tx := range barTxs {
		barTxBz, err := BarTx.ABIEncode(tx)
		assert.NilError(t, err)
		_, err = server.SendMessage(context.Background(), &routerv1.SendMessageRequest{
			Sender:    "hello",
			Message:   barTxBz,
			MessageId: uint64(BarTx.ID()),
		})
		assert.NilError(t, err)
	}

	// we already sent the transactions through to the server, which should pipe them into the transaction queue.
	// all we need to do now is tick so the system can run, which will check that we got the transactions we just
	// piped through above.
	err = w.Tick(context.Background())
	assert.NilError(t, err)
}

func TestServer_Query(t *testing.T) {
	type FooRead struct {
		X uint64
	}
	type FooReply struct {
		Y uint64
	}
	// set up a read that simply returns the FooRead.X
	read := ecs.NewReadType[FooRead, FooReply]("foo", func(world *ecs.World, req FooRead) (FooReply, error) {
		return FooReply{Y: req.X}, nil
	}, ecs.WithReadEVMSupport[FooRead, FooReply])
	w := inmem.NewECSWorldForTest(t)
	err := w.RegisterReads(read)
	assert.NilError(t, err)
	err = w.RegisterTransactions(ecs.NewTransactionType[struct{}, struct{}]("nothing"))
	assert.NilError(t, err)
	s, err := NewServer(w)
	assert.NilError(t, err)

	request := FooRead{X: 3000}
	bz, err := read.EncodeAsABI(request)
	assert.NilError(t, err)

	res, err := s.QueryShard(context.Background(), &routerv1.QueryShardRequest{
		Resource: "foo",
		Request:  bz,
	})
	assert.NilError(t, err)

	gotAny, err := read.DecodeEVMReply(res.Response)
	got := gotAny.(FooReply)
	// Y should equal X here as we simply set reply's Y to request's X in the read handler above.
	assert.Equal(t, got.Y, request.X)
}
