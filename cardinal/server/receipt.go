package server

import (
	"encoding/json"
	"io"
	"net/http"

	"pkg.world.dev/world-engine/cardinal/ecs"
)

type ListTxReceiptsRequest struct {
	StartTick uint64 `json:"start_tick"`
}

// ListTxReceiptsReply returns the transaction receipts for the given range of ticks. The interval is closed on
// StartTick and open on EndTick: i.e. [StartTick, EndTick)
// Meaning StartTick is included and EndTick is not. To iterate over all ticks in the future, use the returned
// EndTick as the StartTick in the next request. If StartTick == EndTick, the receipts list will be empty.
type ListTxReceiptsReply struct {
	StartTick uint64    `json:"start_tick"`
	EndTick   uint64    `json:"end_tick"`
	Receipts  []Receipt `json:"receipts"`
}

// Receipt represents a single transaction receipt. It contains an ID, a result, and a list of errors.
type Receipt struct {
	TxHash string   `json:"tx_hash"`
	Tick   uint64   `json:"tick"`
	Result any      `json:"result"`
	Errors []string `json:"errors"`
}

type TransactionReply struct {
	TxHash string `json:"tx_hash"`
	Tick   uint64 `json:"tick"`
}

func makeListTxReceiptsRequest() ListTxReceiptsRequest {
	return ListTxReceiptsRequest{}
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
func getListTxReceiptsReplyFromRequest(world *ecs.World) func(*ListTxReceiptsRequest) (*ListTxReceiptsReply, error) {
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

func handleListTxReceipts(world *ecs.World) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		buf, err := io.ReadAll(request.Body)
		if err != nil {
			writeError(writer, "unable to ready body", err)
			return
		}
		req, err := decode[ListTxReceiptsRequest](buf)
		if err != nil {
			writeBadRequest(writer, "unable to decode list receipts request", err)
			return
		}

		reply, err := getListTxReceiptsReplyFromRequest(world)(&req)
		if err != nil {
			writeError(writer, "unable to get receipts", err)
			return
		}

		res, err := json.Marshal(reply)
		if err != nil {
			writeError(writer, "unable to marshal response", err)
			return
		}
		writeResult(writer, res)
	}
}
