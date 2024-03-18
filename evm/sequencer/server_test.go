package sequencer

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"pkg.world.dev/world-engine/assert"
	shardv2 "pkg.world.dev/world-engine/rift/shard/v2"
)

// TestMessagesAreOrderedAndProtoMarshalled tests that when messages are sent to and then flushed from the server,
// they are properly ordered and proto marshalled as expected.
func TestMessagesAreOrderedAndProtoMarshalled(t *testing.T) {
	t.Parallel()
	seq := NewShardSequencer()
	namespace := "bruh"
	req := shardv2.SubmitTransactionsRequest{
		Epoch:         10,
		Namespace:     namespace,
		UnixTimestamp: 400,
		Transactions: map[uint64]*shardv2.Transactions{
			44: {
				Txs: []*shardv2.Transaction{
					{
						PersonaTag: "Duncan_Idaho",
						Namespace:  namespace,
						Nonce:      40,
						Signature:  "signature",
						Body:       []byte("some-message"),
					},
				},
			},
			30: {
				Txs: []*shardv2.Transaction{
					{
						PersonaTag: "Paul_Atreides",
						Namespace:  namespace,
						Nonce:      30,
						Signature:  namespace,
						Body:       []byte("some-message"),
					},
				},
			},
		},
	}
	_, err := seq.Submit(context.Background(), &req)
	assert.NilError(t, err)

	flushedMessages, _ := seq.FlushMessages()
	assert.Len(t, flushedMessages, 1)
	messages := flushedMessages[0]
	assert.Len(t, messages.Txs, 2)
	assert.Equal(t, messages.Txs[0].TxId, uint64(30))
	assert.Equal(t, messages.Txs[1].TxId, uint64(44))

	pbMsg := new(shardv2.Transaction)
	err = proto.Unmarshal(messages.Txs[0].GameShardTransaction, pbMsg)
	assert.NilError(t, err)
	assert.Check(t, proto.Equal(pbMsg, req.GetTransactions()[30].GetTxs()[0]))

	err = proto.Unmarshal(messages.Txs[1].GameShardTransaction, pbMsg)
	assert.NilError(t, err)
	assert.Check(t, proto.Equal(pbMsg, req.GetTransactions()[44].GetTxs()[0]))
}

func TestGetBothSlices(t *testing.T) {
	t.Parallel()
	seq := NewShardSequencer()
	_, err := seq.RegisterGameShard(context.Background(), &shardv2.RegisterGameShardRequest{
		Namespace:     "foo",
		RouterAddress: "bar",
	})
	assert.NilError(t, err)

	_, err = seq.Submit(
		context.Background(),
		&shardv2.SubmitTransactionsRequest{
			Epoch:         1,
			UnixTimestamp: 3,
			Namespace:     "foo",
			Transactions: map[uint64]*shardv2.Transactions{
				1: {
					Txs: []*shardv2.Transaction{
						{PersonaTag: "foo", Namespace: "foobar", Nonce: 3},
					},
				},
			},
		})
	assert.NilError(t, err)
	txs, inits := seq.FlushMessages()

	assert.Len(t, txs, 1)
	assert.Len(t, inits, 1)
}
