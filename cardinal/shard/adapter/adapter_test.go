package adapter

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/grpc"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/txpool"
	shardv2 "pkg.world.dev/world-engine/rift/shard/v2"
	"pkg.world.dev/world-engine/sign"
	"testing"
)

type fakeClient struct {
	req *shardv2.SubmitTransactionsRequest
}

var _ shardv2.TransactionHandlerClient = &fakeClient{}

func (f *fakeClient) Submit(_ context.Context, in *shardv2.SubmitTransactionsRequest, _ ...grpc.CallOption) (
	*shardv2.SubmitTransactionsResponse, error,
) {
	f.req = in
	return &shardv2.SubmitTransactionsResponse{}, nil
}

// TestSubmitsCorrectData tests that when the adapter is called, the request to the base shard sequencer is formatted
// correctly.
func TestSubmitsCorrectData(t *testing.T) {
	client := &fakeClient{}
	adapter := adapterImpl{ShardSequencer: client}
	txp := make(txpool.TxMap)
	txp[5] = []txpool.TxData{
		{
			MsgID:  5,
			Msg:    `{foo}`,
			TxHash: "0xfoobar",
			Tx: &sign.Transaction{
				PersonaTag: "foo",
				Namespace:  "bar",
				Nonce:      10,
				Signature:  "foobar",
				Hash:       common.HexToHash("0xfoobar"),
				Body:       nil,
			},
		},
	}
	txp[3] = []txpool.TxData{
		{
			MsgID:  3,
			Msg:    `{bar}`,
			TxHash: "0xfarboo",
			Tx: &sign.Transaction{
				PersonaTag: "bar",
				Namespace:  "foo",
				Nonce:      32,
				Signature:  "foo",
				Hash:       common.HexToHash("0xbar"),
				Body:       nil,
			},
		},
	}
	namespace := "foobar"
	epoch := uint64(120)
	timestamp := uint64(300)
	err := adapter.Submit(context.Background(), txp, namespace, epoch, timestamp)
	assert.NilError(t, err)
	req := client.req
	assert.NotNil(t, req)
	assert.Equal(t, req.Namespace, namespace)
	assert.Equal(t, req.Epoch, epoch)
	assert.Equal(t, req.UnixTimestamp, timestamp)
	assert.Len(t, req.Transactions, 2)
	assert.Len(t, req.Transactions[3].Txs, 1)
	assert.Len(t, req.Transactions[5].Txs, 1)
	assert.Equal(t, req.Transactions[3].Txs[0].Signature, txp[3][0].Tx.Signature)
	assert.Equal(t, req.Transactions[3].Txs[0].PersonaTag, txp[3][0].Tx.PersonaTag)
	assert.Equal(t, req.Transactions[3].Txs[0].Namespace, txp[3][0].Tx.Namespace)
}
