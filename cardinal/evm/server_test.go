package evm

import (
	"context"
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	"pkg.world.dev/world-engine/sign"
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
	w := ecs.NewTestWorld(t)

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

	// need the system to tick for adding the persona + authorized address. so wrapping the test system below
	// in an enabled block to prevent it from checking test cases until the persona transactions are handled.
	enabled := false

	// add a system that checks that they are submitted properly to the world.
	w.AddSystem(func(wCtx ecs.WorldContext) error {
		if !enabled {
			return nil
		}
		inFooTxs := FooTx.In(wCtx)
		inBarTxs := BarTx.In(wCtx)
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

	sender := "0xHelloThere"
	// create authorized addresses for the evm transaction's msg sender.
	ecs.CreatePersonaTx.AddToQueue(w, ecs.CreatePersonaTransaction{
		PersonaTag:    "foo",
		SignerAddress: "bar",
	})
	ecs.AuthorizePersonaAddressTx.AddToQueue(w, ecs.AuthorizePersonaAddress{
		PersonaTag: "foo",
		Address:    sender,
	}, &sign.SignedPayload{PersonaTag: "foo"})
	err := w.Tick(context.Background())
	assert.NilError(t, err)

	// now that the persona transactions are handled, we can flip enabled to true, allowing the test system to run.
	enabled = true

	server, err := NewServer(w)
	assert.NilError(t, err)

	// marshal out the bytes to send from each list of transactions.
	for _, tx := range fooTxs {
		fooTxBz, err := FooTx.ABIEncode(tx)
		assert.NilError(t, err)
		_, err = server.SendMessage(context.Background(), &routerv1.SendMessageRequest{
			Sender:    sender,
			Message:   fooTxBz,
			MessageId: uint64(FooTx.ID()),
		})
		assert.NilError(t, err)
	}
	for _, tx := range barTxs {
		barTxBz, err := BarTx.ABIEncode(tx)
		assert.NilError(t, err)
		_, err = server.SendMessage(context.Background(), &routerv1.SendMessageRequest{
			Sender:    sender,
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
	type FooReq struct {
		X uint64
	}
	type FooReply struct {
		Y uint64
	}
	// set up a query that simply returns the FooReq.X
	query := ecs.NewQueryType[FooReq, FooReply]("foo", func(world *ecs.World, req FooReq) (FooReply, error) {
		return FooReply{Y: req.X}, nil
	}, ecs.WithQueryEVMSupport[FooReq, FooReply])
	w := ecs.NewTestWorld(t)
	err := w.RegisterQueries(query)
	assert.NilError(t, err)
	err = w.RegisterTransactions(ecs.NewTransactionType[struct{}, struct{}]("nothing"))
	assert.NilError(t, err)
	s, err := NewServer(w)
	assert.NilError(t, err)

	request := FooReq{X: 3000}
	bz, err := query.EncodeAsABI(request)
	assert.NilError(t, err)

	res, err := s.QueryShard(context.Background(), &routerv1.QueryShardRequest{
		Resource: "foo",
		Request:  bz,
	})
	assert.NilError(t, err)

	gotAny, err := query.DecodeEVMReply(res.Response)
	got := gotAny.(FooReply)
	// Y should equal X here as we simply set reply's Y to request's X in the query handler above.
	assert.Equal(t, got.Y, request.X)
}

// TestServer_UnauthorizedAddress tests that when a transaction is sent to Cardinal's EVM server, and there is no
// Authorized address for the sender, an error occurs.
func TestServer_UnauthorizedAddress(t *testing.T) {
	// setup the world
	w := ecs.NewTestWorld(t)

	// create the ECS transactions
	FooTx := ecs.NewTransactionType[FooTransaction, TxReply]("footx", ecs.WithTxEVMSupport[FooTransaction, TxReply])

	assert.NilError(t, w.RegisterTransactions(FooTx))

	// create some txs to submit

	fooTx := FooTransaction{X: 420, Y: "world"}

	assert.NilError(t, w.LoadGameState())

	server, err := NewServer(w)
	assert.NilError(t, err)

	fooTxBz, err := FooTx.ABIEncode(fooTx)
	assert.NilError(t, err)

	sender := "hello"
	_, err = server.SendMessage(context.Background(), &routerv1.SendMessageRequest{
		Sender:    sender,
		Message:   fooTxBz,
		MessageId: uint64(FooTx.ID()),
	})
	assert.Error(t, err, fmt.Sprintf("address %s does not have a linked persona tag", sender))
}
