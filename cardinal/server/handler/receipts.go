package handler

import (
	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type ListTxReceiptsRequest struct {
	StartTick uint64 `json:"startTick" mapstructure:"startTick"`
}

// ListTxReceiptsResponse returns the transaction receipts for the given range of ticks. The interval is closed on
// StartTick and open on EndTick: i.e. [StartTick, EndTick)
// Meaning StartTick is included and EndTick is not. To iterate over all ticks in the future, use the returned
// EndTick as the StartTick in the next request. If StartTick == EndTick, the receipts list will be empty.
type ListTxReceiptsResponse struct {
	StartTick uint64         `json:"startTick"`
	EndTick   uint64         `json:"endTick"`
	Receipts  []ReceiptEntry `json:"receipts"`
}

// ReceiptEntry represents a single transaction receipt. It contains an ID, a result, and a list of errors.
type ReceiptEntry struct {
	TxHash string   `json:"txHash"`
	Tick   uint64   `json:"tick"`
	Result any      `json:"result"`
	Errors []string `json:"errors"`
}

// GetReceipts godoc
//
//	@Summary      Retrieves all transaction receipts
//	@Description  Retrieves all transaction receipts
//	@Accept       application/json
//	@Produce      application/json
//	@Param        ListTxReceiptsRequest  body      ListTxReceiptsRequest  true  "Query body"
//	@Success      200                    {object}  ListTxReceiptsResponse "List of receipts"
//	@Failure      400                    {string}  string                 "Invalid request body"
//	@Router       /query/receipts/list [post]
func GetReceipts(wCtx engine.Context) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		req := new(ListTxReceiptsRequest)
		if err := ctx.BodyParser(req); err != nil {
			return err
		}
		reply := ListTxReceiptsResponse{}
		reply.EndTick = wCtx.CurrentTick()
		size := wCtx.ReceiptHistorySize()
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
			currReceipts, err := wCtx.GetTransactionReceiptsForTick(t)
			if err != nil || len(currReceipts) == 0 {
				continue
			}
			for _, r := range currReceipts {
				reply.Receipts = append(reply.Receipts, ReceiptEntry{
					TxHash: string(r.TxHash),
					Tick:   t,
					Result: r.Result,
					Errors: convertErrorsToStrings(r.Errs),
				})
			}
		}
		return ctx.JSON(reply)
	}
}

func convertErrorsToStrings(errs []error) []string {
	if len(errs) == 0 {
		return nil
	}
	result := make([]string, 0, len(errs))
	for _, err := range errs {
		result = append(result, err.Error())
	}
	return result
}
