package iterator

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"pkg.world.dev/world-engine/cardinal/types"
	shardtypes "pkg.world.dev/world-engine/evm/x/shard/types"
	shard "pkg.world.dev/world-engine/rift/shard/v2"
	"pkg.world.dev/world-engine/sign"
)

// Iterator provides functionality to iterate over transactions stored onchain.
//
//go:generate mockgen -source=iterator.go -package mocks -destination=mocks/iterator.go
type Iterator interface {
	// Each calls `fn` for each tick of transactions it queries. An optional "ranges" may be given which will control
	// the start and end ticks queried. If neither are supplied, each will call `fn` from tick 0 to the last tick stored
	// onchain. If only a single number is supplied, `Each` assumes this to be the tick from which to start the queries.
	// If both are supplied, `Each` will call `fn` for ticks ranges[0] and ranges[1] (inclusive).
	Each(fn func(batch []*TxBatch, tick, timestamp uint64) error, ranges ...uint64) error
}

type iterator struct {
	getMsgByID func(id types.MessageID) (types.Message, bool)
	namespace  string
	querier    shardtypes.QueryClient
}

type TxBatch struct {
	Tx       *sign.Transaction
	MsgID    types.MessageID
	MsgValue any
}

func New(
	getMessageByID func(id types.MessageID) (types.Message, bool),
	namespace string,
	querier shardtypes.QueryClient,
) Iterator {
	return &iterator{
		getMsgByID: getMessageByID,
		namespace:  namespace,
		querier:    querier,
	}
}

// Each iterates over txs from the base shard layer. For each batch of transactions found in
// each tick, it will apply the callback function to that batch and it's respective tick and timestamp.
//
//nolint:gocognit // maybe revisit.. idk.
func (t *iterator) Each(
	fn func(batch []*TxBatch, tick, timestamp uint64) error,
	ranges ...uint64,
) error {
	var nextKey []byte
	stopTick := uint64(0)
	if len(ranges) > 0 {
		if ranges[0] > uint64(0) {
			nextKey = makePageKey(ranges[0])
		}
		if len(ranges) > 1 {
			stopTick = ranges[1]
			if ranges[0] > ranges[1] {
				return fmt.Errorf("first number in range must be less than the second (start,stop)")
			}
		}
	}
OuterLoop:
	for {
		res, err := t.querier.Transactions(context.Background(), &shardtypes.QueryTransactionsRequest{
			Namespace: t.namespace,
			Page: &shardtypes.PageRequest{
				Key:   nextKey,
				Limit: 1,
			},
		})
		if err != nil {
			return eris.Wrap(err, "failed to query transactions from base shard")
		}
		for _, epoch := range res.Epochs {
			if stopTick != 0 && epoch.Epoch > stopTick {
				return nil
			}
			tickNumber := epoch.Epoch
			timestamp := epoch.UnixTimestamp
			batches := make([]*TxBatch, 0, len(epoch.Txs))
			for _, tx := range epoch.Txs {
				msgType, exists := t.getMsgByID(types.MessageID(tx.TxId))
				if !exists {
					return eris.Errorf("queried message with ID %d, but it does not exist in Cardinal", tx.TxId)
				}
				protoTx := new(shard.Transaction)
				err := proto.Unmarshal(tx.GameShardTransaction, protoTx)
				if err != nil {
					return eris.Wrap(err, "failed to unmarshal transaction data")
				}
				msgValue, err := msgType.Decode(protoTx.Body)
				if err != nil {
					return err
				}
				batches = append(batches, &TxBatch{
					Tx:       protoTxToSignTx(protoTx),
					MsgID:    msgType.ID(),
					MsgValue: msgValue,
				})
			}
			if err := fn(batches, tickNumber, timestamp); err != nil {
				return err
			}
		}
		if res.Page.Key == nil {
			break OuterLoop
		}
		nextKey = res.Page.Key
	}
	return nil
}

func protoTxToSignTx(t *shard.Transaction) *sign.Transaction {
	tx := &sign.Transaction{
		PersonaTag: t.PersonaTag,
		Namespace:  t.Namespace,
		Nonce:      t.Nonce,
		Signature:  t.Signature,
		Hash:       common.Hash{},
		Body:       t.Body,
	}
	// HashHex will populate the hash.
	tx.HashHex()
	return tx
}

func makePageKey(tick uint64) []byte {
	buf := make([]byte, 8) //nolint: gomnd // its fine.
	binary.BigEndian.PutUint64(buf, tick)
	return buf
}
