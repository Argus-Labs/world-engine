package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/query"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type receiptPlugin struct {
}

func newReceiptPlugin() *receiptPlugin {
	return &receiptPlugin{}
}

var _ Plugin = (*receiptPlugin)(nil)

func (p *receiptPlugin) Register(world *World) error {
	err := p.RegisterQueries(world)
	if err != nil {
		return err
	}
	return nil
}

func (p *receiptPlugin) RegisterQueries(world *World) error {
	err := RegisterQuery[ListTxReceiptsRequest, ListTxReceiptsResponse](world, "list",
		queryReceipts,
		query.WithCustomQueryGroup[ListTxReceiptsRequest, ListTxReceiptsResponse]("receipts"))
	if err != nil {
		return err
	}
	return nil
}

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

// queryReceipts godoc
//
//	@Summary		Get transaction receipts from Cardinal
//	@Description	Get transaction receipts from Cardinal
//	@Accept			application/json
//	@Produce		application/json
//	@Param			ListTxReceiptsRequest	body		ListTxReceiptsRequest	true	"List Transaction Receipts Request"
//	@Success		200						{object}	ListTxReceiptsResponse
//	@Failure		400						{string}	string	"Invalid transaction request"
//	@Router			/query/receipts/list [post]
func queryReceipts(ctx engine.Context, req *ListTxReceiptsRequest) (*ListTxReceiptsResponse, error) {
	reply := ListTxReceiptsResponse{}
	reply.EndTick = ctx.CurrentTick()
	size := ctx.ReceiptHistorySize()
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
		currReceipts, err := ctx.GetTransactionReceiptsForTick(t)
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
	return &reply, nil
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
