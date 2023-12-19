package sequencer

import (
	"google.golang.org/protobuf/proto"
	"pkg.world.dev/world-engine/assert"
	shardv2 "pkg.world.dev/world-engine/rift/shard/v2"
	"testing"
)

// TestMessagesAreOrderedAndProtoMarshalled tests that when messages are sent to and then flushed from the server,
// they are properly ordered and proto marshalled as expected.
func TestMessagesAreOrderedAndProtoMarshalled(t *testing.T) {
	t.Parallel()
	seq := NewShardSequencer()
	namespace := "bruh"
	req := shardv2.SubmitTransactionsRequest{
		Epoch:     10,
		Namespace: namespace,
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
	_, err := seq.Submit(nil, &req)
	assert.NilError(t, err)

	flushedMessages := seq.FlushMessages()
	assert.Len(t, flushedMessages, 1)
	messages := flushedMessages[0]
	assert.Len(t, messages.Txs, 2)
	assert.Equal(t, messages.Txs[0].TxId, uint64(30))
	assert.Equal(t, messages.Txs[1].TxId, uint64(44))

	pbMsg := new(shardv2.Transaction)
	err = proto.Unmarshal(messages.Txs[0].GameShardTransaction, pbMsg)
	assert.NilError(t, err)
	assert.Check(t, proto.Equal(pbMsg, req.Transactions[30].Txs[0]))

	err = proto.Unmarshal(messages.Txs[1].GameShardTransaction, pbMsg)
	assert.NilError(t, err)
	assert.Check(t, proto.Equal(pbMsg, req.Transactions[44].Txs[0]))
}
