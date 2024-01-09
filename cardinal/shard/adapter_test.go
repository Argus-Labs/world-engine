package shard

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/grpc"
	"pkg.world.dev/world-engine/cardinal/txpool"
	shardv2 "pkg.world.dev/world-engine/rift/shard/v2"
	"pkg.world.dev/world-engine/sign"
	"testing"
)

type fakeClient struct {
	req *shardv2.SubmitTransactionsRequest
}

var _ shardv2.TransactionHandlerClient = fakeClient{}

func (f fakeClient) Submit(ctx context.Context, in *shardv2.SubmitTransactionsRequest, opts ...grpc.CallOption) (*shardv2.SubmitTransactionsResponse, error) {
	return nil, nil
}

func TestSubmit(t *testing.T) {
	adapter := adapterImpl{ShardSequencer: fakeClient{}}
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
	err := adapter.Submit(context.Background(), txp, "foobar", 120, 320)

}
