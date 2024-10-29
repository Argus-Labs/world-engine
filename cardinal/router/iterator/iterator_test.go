package iterator_test

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/router/iterator"
	"pkg.world.dev/world-engine/cardinal/types/message"
	shard "pkg.world.dev/world-engine/rift/shard/v2"
)

var _ shard.TransactionHandlerClient = &mockQuerier{}
var fooMsg = message.NewMessageType[fooIn]()

type fooIn struct{ X int }

func (fooIn) Name() string { return "foo" }

type mockQuerier struct {
	i       int
	retErr  error
	ret     []*shard.QueryTransactionsResponse
	request *shard.QueryTransactionsRequest
}

func (m *mockQuerier) RegisterGameShard(_ context.Context, _ *shard.RegisterGameShardRequest, _ ...grpc.CallOption) (
	*shard.RegisterGameShardResponse, error,
) {
	panic("intentionally not implemented. this is a mock.")
}

func (m *mockQuerier) Submit(_ context.Context, _ *shard.SubmitTransactionsRequest, _ ...grpc.CallOption) (
	*shard.SubmitTransactionsResponse, error,
) {
	panic("intentionally not implemented. this is a mock.")
}

// this mock will return its error, if set, otherwise, it will return whatever is in ret[i], where i represents the
// amount of times this was called.
func (m *mockQuerier) QueryTransactions(
	_ context.Context,
	req *shard.QueryTransactionsRequest,
	_ ...grpc.CallOption,
) (*shard.QueryTransactionsResponse, error) {
	m.request = req
	if m.retErr != nil {
		return nil, m.retErr
	}
	defer func() { m.i++ }()
	return m.ret[m.i], nil
}

func TestIteratorHappyPath(t *testing.T) {
	namespace := "ns"
	msgValue := fooIn{3}
	msgBytes, err := fooMsg.Encode(msgValue)
	assert.NilError(t, err)
	protoTx := &shard.Transaction{
		PersonaTag: "ty",
		Namespace:  namespace,
		Nonce:      1,
		Signature:  "fo",
		Body:       msgBytes,
	}
	txBz, err := proto.Marshal(protoTx)
	assert.NilError(t, err)
	querier := &mockQuerier{
		ret: []*shard.QueryTransactionsResponse{
			{
				Epochs: []*shard.Epoch{
					{
						Epoch:         12,
						UnixTimestamp: 15,
						Txs: []*shard.TxData{
							{
								TxId:                 fooMsg.Name(),
								GameShardTransaction: txBz,
							},
						},
					},
				},
				Page: &shard.PageResponse{},
			},
		},
	}
	it := iterator.New(
		namespace,
		querier,
	)
	err = it.Each(func(batch []*iterator.TxBatch, tick, timestamp uint64) error {
		assert.Len(t, batch, 1)
		assert.Equal(t, tick, uint64(12))
		assert.Equal(t, timestamp, uint64(15))
		tx := batch[0]

		assert.Equal(t, tx.MsgName, fooMsg.Name())
		assert.Equal(t, tx.Tx.PersonaTag, protoTx.GetPersonaTag())
		assert.True(t, len(tx.Tx.Hash.Bytes()) > 1)
		assert.Equal(t, tx.Tx.Namespace, namespace)
		assert.DeepEqual(t, []byte(tx.Tx.Body), msgBytes)

		return nil
	})
	assert.NilError(t, err)
}

func TestIteratorStartRange(t *testing.T) {
	querier := &mockQuerier{retErr: errors.New("whatever")}
	it := iterator.New("", querier)

	// we dont care about this error, we're just checking if `querier` gets called with the right key in the Page.
	startRange := uint64(5)
	_ = it.Each(nil, 5)

	req := querier.request
	gotStartRange := parsePageKey(req.GetPage().GetKey())
	assert.Equal(t, startRange, gotStartRange)
}

func TestIteratorStopRange(t *testing.T) {
	namespace := "ns"
	msgValue := fooIn{3}
	msgBytes, err := fooMsg.Encode(msgValue)
	assert.NilError(t, err)
	protoTx := &shard.Transaction{
		PersonaTag: "ty",
		Namespace:  namespace,
		Nonce:      1,
		Signature:  "fo",
		Body:       msgBytes,
	}
	txBz, err := proto.Marshal(protoTx)
	assert.NilError(t, err)
	querier := &mockQuerier{
		ret: []*shard.QueryTransactionsResponse{
			{
				Epochs: []*shard.Epoch{
					{
						Epoch:         12,
						UnixTimestamp: 15,
						Txs: []*shard.TxData{
							{
								TxId:                 fooMsg.Name(),
								GameShardTransaction: txBz,
							},
						},
					},
					{
						Epoch: 20,
					},
				},
				Page: &shard.PageResponse{},
			},
		},
	}
	it := iterator.New(
		namespace,
		querier,
	)
	called := 0
	err = it.Each(func(_ []*iterator.TxBatch, _, _ uint64) error {
		called++
		return nil
	}, 0, 15)
	assert.NilError(t, err)
	assert.Equal(t, called, 1)
}

func TestStartGreaterThanStopRange(t *testing.T) {
	it := iterator.New("", nil)
	err := it.Each(nil, 154, 0)
	assert.ErrorContains(t, err, "first number in range must be less than the second (start,stop)")
}

func parsePageKey(key []byte) uint64 {
	tick := binary.BigEndian.Uint64(key)
	return tick
}
