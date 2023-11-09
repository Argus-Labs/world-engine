package evm

import (
	"context"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
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
	fooTx := ecs.NewMessageType[FooTransaction, TxReply]("footx", ecs.WithMsgEVMSupport[FooTransaction, TxReply])
	barTx := ecs.NewMessageType[BarTransaction, TxReply]("bartx", ecs.WithMsgEVMSupport[BarTransaction, TxReply])

	assert.NilError(t, w.RegisterMessages(fooTx, barTx))

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

	currFooIndex := 0
	currBarIndex := 0

	// add a system that checks that they are submitted properly to the world in the correct order.
	w.RegisterSystem(func(wCtx ecs.WorldContext) error {
		inFooTxs := fooTx.In(wCtx)
		inBarTxs := barTx.In(wCtx)
		if len(inFooTxs) == 0 && len(inBarTxs) == 0 {
			return nil
		}
		// Make sure we're only dealing with 1 transaction
		assert.Equal(t, 1, len(inFooTxs)+len(inBarTxs))

		if len(inFooTxs) > 0 {
			assert.Check(t, cmp.DeepEqual(inFooTxs[0].Msg, fooTxs[currFooIndex]))
			currFooIndex++
		} else {
			assert.Check(t, cmp.DeepEqual(inBarTxs[0].Msg, barTxs[currBarIndex]))
			currBarIndex++

			// Make sure we process the bar transaction AFTER all the foo transactions
			assert.Check(t, len(fooTxs) == currFooIndex, "%d is not %d", len(inFooTxs), currFooIndex)
		}
		return nil
	})
	assert.NilError(t, w.LoadGameState())

	sender := "0xHelloThere"
	personaTag := "foo"
	// create authorized addresses for the evm transaction's msg sender.
	ecs.CreatePersonaMsg.AddToQueue(w, ecs.CreatePersona{
		PersonaTag:    personaTag,
		SignerAddress: "bar",
	})
	ecs.AuthorizePersonaAddressMsg.AddToQueue(w, ecs.AuthorizePersonaAddress{
		Address: sender,
	}, &sign.Transaction{PersonaTag: personaTag})
	tickStartCh := make(chan time.Time)
	tickDoneCh := make(chan uint64)
	w.StartGameLoop(context.Background(), tickStartCh, tickDoneCh)
	// Process an initial tick so the CreatePersona transaction will be processed
	tickStartCh <- time.Now()
	<-tickDoneCh

	server, err := NewServer(w)
	assert.NilError(t, err)

	txSequenceDone := make(chan struct{})
	// Send in each transaction one at a time
	go func() {
		// marshal out the bytes to send from each list of transactions.
		for _, tx := range fooTxs {
			var fooTxBz []byte
			fooTxBz, err = fooTx.ABIEncode(tx)
			assert.NilError(t, err)
			_, err = server.SendMessage(context.Background(), &routerv1.SendMessageRequest{
				Sender:    sender,
				Message:   fooTxBz,
				MessageId: fooTx.Name(),
			})
			assert.NilError(t, err)
		}
		for _, tx := range barTxs {
			var barTxBz []byte
			barTxBz, err = barTx.ABIEncode(tx)
			assert.NilError(t, err)
			_, err = server.SendMessage(context.Background(), &routerv1.SendMessageRequest{
				Sender:    sender,
				Message:   barTxBz,
				MessageId: barTx.Name(),
			})
			assert.NilError(t, err)
		}
		close(txSequenceDone)
	}()

loop:
	for {
		select {
		case tickStartCh <- time.Now():
			<-tickDoneCh
		case <-txSequenceDone:
			break loop
		}
	}
	// Make sure we saw the correct number of foo and bar transactions
	assert.Equal(t, len(fooTxs), currFooIndex)
	assert.Equal(t, len(barTxs), currBarIndex)
}

func TestServer_Query(t *testing.T) {
	type FooReq struct {
		X uint64
	}
	type FooReply struct {
		Y uint64
	}
	// set up a query that simply returns the FooReq.X
	query := ecs.NewQueryType[FooReq, FooReply]("foo", func(wCtx ecs.WorldContext, req FooReq) (FooReply, error) {
		return FooReply{Y: req.X}, nil
	}, ecs.WithQueryEVMSupport[FooReq, FooReply])
	w := ecs.NewTestWorld(t)
	err := w.RegisterQueries(query)
	assert.NilError(t, err)
	err = w.RegisterMessages(ecs.NewMessageType[struct{}, struct{}]("nothing"))
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
	assert.NilError(t, err)
	got, ok := gotAny.(FooReply)
	assert.Equal(t, ok, true)
	// Y should equal X here as we simply set reply's Y to request's X in the query handler above.
	assert.Equal(t, got.Y, request.X)
}

// TestServer_UnauthorizedAddress tests that when a transaction is sent to Cardinal's EVM server, and there is no
// Authorized address for the sender, an error occurs.
func TestServer_UnauthorizedAddress(t *testing.T) {
	// setup the world
	w := ecs.NewTestWorld(t)

	// create the ECS transactions
	fooTxType := ecs.NewMessageType[FooTransaction, TxReply]("footx", ecs.WithMsgEVMSupport[FooTransaction, TxReply])

	assert.NilError(t, w.RegisterMessages(fooTxType))

	// create some txs to submit

	fooTx := FooTransaction{X: 420, Y: "world"}

	assert.NilError(t, w.LoadGameState())

	server, err := NewServer(w)
	assert.NilError(t, err)

	fooTxBz, err := fooTxType.ABIEncode(fooTx)
	assert.NilError(t, err)

	sender := "hello"
	// server will never error. always returns it in the result.
	res, _ := server.SendMessage(context.Background(), &routerv1.SendMessageRequest{
		Sender:    sender,
		Message:   fooTxBz,
		MessageId: fooTxType.Name(),
	})
	assert.Equal(t, res.Code, uint32(CodeUnauthorized))
	assert.Check(t, strings.Contains(res.Errs, "failed to authorize"))
}
