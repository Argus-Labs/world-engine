package server1

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type ListTxReceiptsRequest struct {
	StartTick uint64 `json:"startTick" mapstructure:"startTick"`
}

// ListTxReceiptsReply returns the transaction receipts for the given range of ticks. The interval is closed on
// StartTick and open on EndTick: i.e. [StartTick, EndTick)
// Meaning StartTick is included and EndTick is not. To iterate over all ticks in the future, use the returned
// EndTick as the StartTick in the next request. If StartTick == EndTick, the receipts list will be empty.
type ListTxReceiptsReply struct {
	StartTick uint64    `json:"startTick"`
	EndTick   uint64    `json:"endTick"`
	Receipts  []Receipt `json:"receipts"`
}

// Receipt represents a single transaction receipt. It contains an ID, a result, and a list of errors.
type Receipt struct {
	TxHash string   `json:"txHash"`
	Tick   uint64   `json:"tick"`
	Result any      `json:"result"`
	Errors []string `json:"errors"`
}

type TransactionReply struct {
	TxHash string `json:"txHash"`
	Tick   uint64 `json:"tick"`
}

// errsToStringSlice convert a slice of errors into a slice of strings. This is needed as json.Marshal does not
// extract the Error string from errors when marshalling.
func errsToStringSlice(errs []error) []string {
	r := make([]string, 0, len(errs))
	for _, err := range errs {
		r = append(r, err.Error())
	}
	return r
}

// with world construct a function that takes a receipts request and returns a reply.
func getListTxReceiptsReplyFromRequest(world *ecs.Engine) func(*ListTxReceiptsRequest) (*ListTxReceiptsReply, error) {
	return func(req *ListTxReceiptsRequest) (*ListTxReceiptsReply, error) {
		reply := ListTxReceiptsReply{}
		reply.EndTick = world.CurrentTick()
		size := world.ReceiptHistorySize()
		if size > reply.EndTick {
			reply.StartTick = 0
		} else {
			reply.StartTick = reply.EndTick - size
		}
		// StartTick and EndTick are now at the largest possible range of ticks.
		// Check to see if we should narrow down the range at all.
		if req.StartTick > reply.EndTick {
			// User is asking for ticks in the future.
			reply.StartTick = reply.EndTick
		} else if req.StartTick > reply.StartTick {
			reply.StartTick = req.StartTick
		}

		for t := reply.StartTick; t < reply.EndTick; t++ {
			currReceipts, err := world.GetTransactionReceiptsForTick(t)
			if err != nil || len(currReceipts) == 0 {
				continue
			}
			for _, r := range currReceipts {
				reply.Receipts = append(reply.Receipts, Receipt{
					TxHash: string(r.TxHash),
					Tick:   t,
					Result: r.Result,
					Errors: errsToStringSlice(r.Errs),
				})
			}
		}
		return &reply, nil
	}
}
