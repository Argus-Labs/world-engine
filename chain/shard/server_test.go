package shard

import (
	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"context"
	"gotest.tools/v3/assert"
	"testing"
)

func TestServer(t *testing.T) {
	s := NewShardServer()
	ctx := context.Background()
	_, err := s.SubmitCardinalTx(ctx, &shardv1.SubmitCardinalTxRequest{
		Tick: 1,
		TxId: 1,
		Tx: &shardv1.SignedPayload{
			PersonaTag: "hi",
			Namespace:  "foobar",
			Nonce:      4,
			Signature:  "sig",
			Body:       []byte("hi"),
		},
	})
	assert.NilError(t, err)
	_, err = s.SubmitCardinalTx(ctx, &shardv1.SubmitCardinalTxRequest{
		Tick: 1,
		TxId: 2,
		Tx: &shardv1.SignedPayload{
			PersonaTag: "hi",
			Namespace:  "foobar",
			Nonce:      5,
			Signature:  "sig",
			Body:       []byte("x1"),
		},
	})
	assert.NilError(t, err)
	_, err = s.SubmitCardinalTx(ctx, &shardv1.SubmitCardinalTxRequest{
		Tick: 40,
		TxId: 2,
		Tx: &shardv1.SignedPayload{
			PersonaTag: "hi",
			Namespace:  "barfoo",
			Nonce:      5,
			Signature:  "sig",
			Body:       []byte("x1"),
		},
	})
	assert.NilError(t, err)

	txs := s.FlushMessages()
	// outbox should not yet have anything in it, as the server has not yet received a new tick,
	// so it will wait for more txs from it's currently known tick.
	assert.Equal(t, len(txs), 0)
	// submit a tx in namespace with the next tick.
	_, err = s.SubmitCardinalTx(ctx, &shardv1.SubmitCardinalTxRequest{
		Tick: 4,
		TxId: 2,
		Tx: &shardv1.SignedPayload{
			PersonaTag: "hi",
			Namespace:  "foobar",
			Nonce:      5,
			Signature:  "sig",
			Body:       []byte("x1"),
		},
	})
	assert.NilError(t, err)
	txs = s.FlushMessages()
	// txs should now have 1 request.
	assert.Equal(t, len(txs), 1)
	// the request should have the 2 transactions from the first tick.
	assert.Equal(t, len(txs[0].Txs.Txs), 2)
}
