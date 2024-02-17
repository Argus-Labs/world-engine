package receipt

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"pkg.world.dev/world-engine/relay/nakama/utils"
	"sync"
	"time"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
)

const (
	TransactionReceiptsEndpoint = "query/receipts/list"
)

type TransactionReceiptsReply struct {
	StartTick uint64     `json:"startTick"`
	EndTick   uint64     `json:"endTick"`
	Receipts  []*Receipt `json:"receipts"`
}

type Receipt struct {
	TxHash string         `json:"txHash"`
	Result map[string]any `json:"result"`
	Errors []string       `json:"errors"`
}

// Dispatcher continually polls Cardinal for transaction receipts and dispatches them to any subscribed
// channels. The subscribed channels are stored in the sync.Map.
type Dispatcher struct {
	ch chan []*Receipt
	m  *sync.Map
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		ch: make(chan []*Receipt),
		m:  &sync.Map{},
	}
}

// Subscribe allows for the sending of receipts to the given channel. Each given session can
// only be associated with a single channel.
func (r *Dispatcher) Subscribe(session string, ch chan []*Receipt) {
	r.m.Store(session, ch)
}

// Dispatch continually drains r.ch (receipts from cardinal) and sends copies to all subscribed channels.
// This function is meant to be called in a goroutine. Pushed receipts will not block when sending.
func (r *Dispatcher) Dispatch(logger runtime.Logger) {
	for receipts := range r.ch {
		r.m.Range(func(key, value any) bool {
			ch, _ := value.(chan []*Receipt)
			// avoid blocking r.ch by making a best-effort delivery here.
			select {
			case ch <- receipts:
			default:
				logger.Info("session %s dropped a batch of %d receipts", key, len(receipts))
			}
			return true
		})
	}
}

// PollReceipts calls the cardinal backend to get any new transaction receipts. It never returns, so
// it should be called in a goroutine.
func (r *Dispatcher) PollReceipts(log runtime.Logger, cardinalAddr string) {
	timeBetweenBatched := time.Second
	startTick := uint64(0)
	var err error
	log.Debug("fetching batch of receipts: %d", startTick)
	for {
		startTick, err = r.streamBatchOfReceipts(log, startTick, cardinalAddr)
		if err != nil {
			log.Error("problem when fetching batch of receipts: %v", eris.ToString(eris.Wrap(err, ""), true))
		}
		time.Sleep(timeBetweenBatched)
	}
}

func (r *Dispatcher) streamBatchOfReceipts(
	_ runtime.Logger,
	startTick uint64,
	cardinalAddr string,
) (newStartTick uint64, err error) {
	newStartTick = startTick
	reply, err := r.getBatchOfReceiptsFromCardinal(startTick, cardinalAddr)
	if err != nil {
		return newStartTick, err
	}
	r.ch <- reply.Receipts
	return reply.EndTick, nil
}

type txReceiptRequest struct {
	StartTick uint64 `json:"startTick"`
}

func (r *Dispatcher) getBatchOfReceiptsFromCardinal(startTick uint64, cardinalAddr string) (
	reply *TransactionReceiptsReply, err error) {
	request := txReceiptRequest{
		StartTick: startTick,
	}
	buf, err := json.Marshal(request)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	ctx := context.Background()
	url := utils.MakeHTTPURL(TransactionReceiptsEndpoint, cardinalAddr)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := utils.DoRequest(req)
	if err != nil {
		return nil, eris.Wrapf(err, "failed to query %q", url)
	}
	defer resp.Body.Close()

	reply = &TransactionReceiptsReply{}

	if err = json.NewDecoder(resp.Body).Decode(reply); err != nil {
		return nil, eris.Wrap(err, "")
	}
	return reply, nil
}
