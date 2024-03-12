package events

import (
	"encoding/json"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/receipt"
)

type TickResults struct {
	Tick     uint64
	Receipts []receipt.Receipt
	Events   [][]byte
}

func NewTickResults(initialTick uint64) *TickResults {
	return &TickResults{
		Tick:     initialTick,
		Receipts: []receipt.Receipt{},
		Events:   [][]byte{},
	}
}

func (tr *TickResults) AddEvent(event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return eris.Wrap(err, "must use a json serializable type for emitting events")
	}
	tr.Events = append(tr.Events, data)
	return nil
}

func (tr *TickResults) SetReceipts(newReceipts []receipt.Receipt) {
	tr.Receipts = newReceipts
}

func (tr *TickResults) SetTick(tick uint64) {
	tr.Tick = tick
}

func (tr *TickResults) Clear() {
	tr.Tick = 0
	tr.Receipts = nil
	tr.Events = nil
}
