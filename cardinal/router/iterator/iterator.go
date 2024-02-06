package iterator

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"pkg.world.dev/world-engine/cardinal/types/message"
	shardtypes "pkg.world.dev/world-engine/evm/x/shard/types"
	shard "pkg.world.dev/world-engine/rift/shard/v2"
	"pkg.world.dev/world-engine/sign"
)

type Iterator interface {
	Each(fn func(batch []*TxBatch, tick, timestamp uint64) error) error
}

type iterator struct {
	getMsgById func(message.TypeID) (message.Message, bool)
	namespace  string
	querier    shardtypes.QueryClient
}

type TxBatch struct {
	Tx       *sign.Transaction
	MsgID    message.TypeID
	MsgValue any
}

func NewIterator(getMessageById func(id message.TypeID) (message.Message, bool), namespace string, querier shardtypes.QueryClient) Iterator {
	return &iterator{
		getMsgById: getMessageById,
		namespace:  namespace,
		querier:    querier,
	}
}

func (t *iterator) Each(fn func(batch []*TxBatch, tick, timestamp uint64) error) error {
	var nextKey []byte
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
			tickNumber := epoch.Epoch
			timestamp := epoch.UnixTimestamp
			batches := make([]*TxBatch, 0, len(epoch.Txs))
			for _, tx := range epoch.Txs {
				msgType, exists := t.getMsgById(message.TypeID(tx.TxId))
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
