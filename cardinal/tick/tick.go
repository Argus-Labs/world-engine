package tick

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/types"
)

type Proposal struct {
	ID        int64
	Timestamp int64
	Namespace types.Namespace
	Txs       types.TxMap
}

type Tick struct {
	*Proposal
	Receipts map[common.Hash]Receipt
	Events   map[string][]any
}

func New(proposal *Proposal) (*Tick, error) {
	if proposal == nil {
		return nil, eris.New("proposal cannot be nil")
	}
	return &Tick{
		Proposal: proposal,
		Receipts: make(map[common.Hash]Receipt),
		Events:   make(map[string][]any),
	}, nil
}

// SetReceipts sets the given transaction hash to the given result.
// Calling this multiple times will replace any previous results.
func (t *Tick) SetReceipts(hash common.Hash, result any, txErr error) error {
	rec, ok := t.Receipts[hash]
	if !ok {
		rec = Receipt{}
	}

	resultBz, err := json.Marshal(result)
	if err != nil {
		return err
	}

	rec.TxHash = hash
	rec.Result = resultBz

	if txErr != nil {
		rec.Error = txErr.Error()
	} else {
		rec.Error = ""
	}

	t.Receipts[hash] = rec

	log.Info().Str("tx_hash", hash.Hex()).RawJSON("result", resultBz).Msg("Set receipt")
	return nil
}

func (t *Tick) RecordEvent(systemName string, event any) {
	t.Events[systemName] = append(t.Events[systemName], event)
}
