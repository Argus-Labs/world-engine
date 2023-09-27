package ecb

import (
	"context"

	"github.com/redis/go-redis/v9"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/sign"
)

// The world tick must be updated in the same atomic transaction as all the state changes
// associated with that tick. This means the manager here must also implement the TickStore interface.
var _ storage.TickStorage = &Manager{}

func (m *Manager) GetTickNumbers() (start, end uint64, err error) {
	ctx := context.Background()
	start, err = m.client.Get(ctx, redisStartTickKey()).Uint64()
	if err == redis.Nil {
		start = 0
	} else if err != nil {
		return 0, 0, err
	}
	end, err = m.client.Get(ctx, redisEndTickKey()).Uint64()
	if err == redis.Nil {
		end = 0
	} else if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

func (m *Manager) StartNextTick(txs []transaction.ITransaction, queue *transaction.TxQueue) error {
	ctx := context.Background()
	pipe := m.client.TxPipeline()
	if err := addPendingTransactionToPipe(ctx, pipe, txs, queue); err != nil {
		return err
	}

	if err := pipe.Incr(ctx, redisStartTickKey()).Err(); err != nil {
		return err
	}

	_, err := pipe.Exec(ctx)
	return err
}

func (m *Manager) FinalizeTick() error {
	ctx := context.Background()
	pipe, err := m.makePipeOfRedisCommands(ctx)
	if err != nil {
		return err
	}
	if err = pipe.Incr(context.Background(), redisEndTickKey()).Err(); err != nil {
		return err
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (m *Manager) Recover(txs []transaction.ITransaction) (*transaction.TxQueue, error) {
	ctx := context.Background()
	key := redisPendingTransactionKey()
	bz, err := m.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	pending, err := codec.Decode[[]pendingTransaction](bz)
	if err != nil {
		return nil, err
	}
	idToTx := map[transaction.TypeID]transaction.ITransaction{}
	for _, tx := range txs {
		idToTx[tx.ID()] = tx
	}

	txQueue := transaction.NewTxQueue()
	for _, p := range pending {
		tx := idToTx[p.TypeID]
		txData, err := tx.Decode(p.Data)
		if err != nil {
			return nil, err
		}
		txQueue.AddTransaction(tx.ID(), txData, p.Sig)
	}
	return txQueue, nil
}

type pendingTransaction struct {
	TypeID transaction.TypeID
	TxHash transaction.TxHash
	Data   []byte
	Sig    *sign.SignedPayload
}

func addPendingTransactionToPipe(ctx context.Context, pipe redis.Pipeliner, txs []transaction.ITransaction, queue *transaction.TxQueue) error {
	var pending []pendingTransaction
	for _, tx := range txs {
		currList := queue.ForID(tx.ID())
		for _, txData := range currList {
			buf, err := tx.Encode(txData.Value)
			if err != nil {
				return err
			}
			currItem := pendingTransaction{
				TypeID: tx.ID(),
				TxHash: txData.TxHash,
				Sig:    txData.Sig,
				Data:   buf,
			}
			pending = append(pending, currItem)
		}
	}
	buf, err := codec.Encode(pending)
	if err != nil {
		return err
	}
	key := redisPendingTransactionKey()
	return pipe.Set(ctx, key, buf, 0).Err()
}
