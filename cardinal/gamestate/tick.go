package gamestate

import (
	"context"
	"pkg.world.dev/world-engine/cardinal/codec"
	"pkg.world.dev/world-engine/cardinal/types"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/statsd"
	"pkg.world.dev/world-engine/cardinal/txpool"

	"github.com/redis/go-redis/v9"
	"pkg.world.dev/world-engine/sign"
)

// The engine tick must be updated in the same atomic transaction as all the state changes
// associated with that tick. This means the manager here must also implement the TickStore interface.
var _ TickStorage = &EntityCommandBuffer{}

// GetTickNumbers returns the last tick that was started and the last tick that was ended. If start == end, it means
// the last tick that was attempted completed successfully. If start != end, it means a tick was started but did not
// complete successfully; Recover must be used to recover the pending transactions so the previously started tick can
// be completed.
func (m *EntityCommandBuffer) GetTickNumbers() (start, end uint64, err error) {
	ctx := context.Background()
	start, err = m.client.Get(ctx, redisStartTickKey()).Uint64()
	err = eris.Wrap(err, "")
	if eris.Is(eris.Cause(err), redis.Nil) {
		start = 0
	} else if err != nil {
		return 0, 0, err
	}
	end, err = m.client.Get(ctx, redisEndTickKey()).Uint64()
	err = eris.Wrap(err, "")
	if eris.Is(eris.Cause(err), redis.Nil) {
		end = 0
	} else if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

// StartNextTick saves the given transactions to the DB and sets the tick trackers to indicate we are in the middle
// of a tick. While transactions are saved to the DB, no state changes take place at this time.
func (m *EntityCommandBuffer) StartNextTick(txs []types.Message, pool *txpool.TxPool) error {
	ctx := context.Background()
	pipe := m.client.TxPipeline()
	if err := addPendingTransactionToPipe(ctx, pipe, txs, pool); err != nil {
		return err
	}

	if err := pipe.Incr(ctx, redisStartTickKey()).Err(); err != nil {
		return eris.Wrap(err, "")
	}

	_, err := pipe.Exec(ctx)
	return eris.Wrap(err, "")
}

// FinalizeTick combines all pending state changes into a single multi/exec redis transactions and commits them
// to the DB.
func (m *EntityCommandBuffer) FinalizeTick(ctx context.Context) error {
	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "tick.span.finalize")
	defer func() {
		span.Finish()
	}()
	makePipeStartTime := time.Now()
	pipe, err := m.makePipeOfRedisCommands(ctx)
	if err != nil {
		return err
	}
	if err = pipe.Incr(ctx, redisEndTickKey()).Err(); err != nil {
		return eris.Wrap(err, "")
	}
	statsd.EmitTickStat(makePipeStartTime, "pipe_make")
	flushStartTime := time.Now()
	_, err = pipe.Exec(ctx)
	statsd.EmitTickStat(flushStartTime, "pipe_exec")
	if err != nil {
		return eris.Wrap(err, "")
	}

	m.pendingArchIDs = nil
	m.DiscardPending()
	return nil
}

// Recover fetches the pending transactions for an incomplete tick. This should only be called if GetTickNumbers
// indicates that the previous tick was started, but never completed.
func (m *EntityCommandBuffer) Recover(txs []types.Message) (*txpool.TxPool, error) {
	ctx := context.Background()
	key := redisPendingTransactionKey()
	bz, err := m.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	pending, err := codec.Decode[[]pendingTransaction](bz)
	if err != nil {
		return nil, err
	}
	idToTx := map[types.MessageID]types.Message{}
	for _, tx := range txs {
		idToTx[tx.ID()] = tx
	}

	txPool := txpool.New()
	for _, p := range pending {
		tx := idToTx[p.TypeID]
		var txData any
		txData, err = tx.Decode(p.Data)
		if err != nil {
			return nil, err
		}
		txPool.AddTransaction(tx.ID(), txData, p.Tx)
	}
	return txPool, nil
}

type pendingTransaction struct {
	TypeID types.MessageID
	TxHash types.TxHash
	Data   []byte
	Tx     *sign.Transaction
}

func addPendingTransactionToPipe(
	ctx context.Context, pipe redis.Pipeliner, txs []types.Message,
	pool *txpool.TxPool,
) error {
	var pending []pendingTransaction
	for _, tx := range txs {
		currList := pool.ForID(tx.ID())
		for _, txData := range currList {
			buf, err := tx.Encode(txData.Msg)
			if err != nil {
				return err
			}
			currItem := pendingTransaction{
				TypeID: tx.ID(),
				TxHash: txData.TxHash,
				Tx:     txData.Tx,
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
	return eris.Wrap(pipe.Set(ctx, key, buf, 0).Err(), "")
}
