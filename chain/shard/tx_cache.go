package shard

import (
	"github.com/argus-labs/world-engine/chain/x/shard/types"
)

type TxCache struct {
	ticksToBeProcessed []uint64
	txs                map[uint64]*types.SubmitCardinalTxRequest
}
