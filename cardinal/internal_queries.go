package cardinal

import (
	"encoding/json"
	"pkg.world.dev/world-engine/cardinal/cql"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

type DebugRequest struct{}

type debugStateElement struct {
	ID         entity.ID         `json:"id"`
	Components []json.RawMessage `json:"components" swaggertype:"array,object"`
}

type DebugStateResponse []*debugStateElement

// queryDebugState godoc
//
//	@Summary		Get information on all entities and components in world-engine
//	@Description	Displays the entire game state.
//	@Produce		application/json
//	@Success		200	{object}	DebugStateResponse
//	@Router			/query/debug/state [post]
func queryDebugState(ctx engine.Context, _ *DebugRequest) (*DebugStateResponse, error) {
	result := make(DebugStateResponse, 0)
	s := NewSearch(ctx, filter.All())
	var eachClosureErr error
	searchEachErr := s.Each(
		func(id entity.ID) bool {
			var components []component.ComponentMetadata
			components, eachClosureErr = ctx.StoreReader().GetComponentTypesForEntity(id)
			if eachClosureErr != nil {
				return false
			}
			resultElement := debugStateElement{
				ID:         id,
				Components: make([]json.RawMessage, 0),
			}
			for _, c := range components {
				var data json.RawMessage
				data, eachClosureErr = ctx.StoreReader().GetComponentForEntityInRawJSON(c, id)
				if eachClosureErr != nil {
					return false
				}
				resultElement.Components = append(resultElement.Components, data)
			}
			result = append(result, &resultElement)
			return true
		},
	)
	if eachClosureErr != nil {
		return nil, eachClosureErr
	}
	if searchEachErr != nil {
		return nil, searchEachErr
	}

	return &result, nil
}

type CQLQueryRequest struct {
	CQL string
}

type cqlData struct {
	ID   entity.ID         `json:"id"`
	Data []json.RawMessage `json:"data" swaggertype:"object"`
}

type CQLQueryResponse struct {
	Results []cqlData `json:"results"`
}

// queryCQL godoc
// @Summary		Query the ecs with CQL (cardinal query language)
// @Description	Query the ecs with CQL (cardinal query language)
// @Accept			application/json
// @Produce		application/json
// @Param			cql	body		CQLQueryRequest	true	"cql (cardinal query language)"
// @Success		200	{object}	CQLQueryResponse
// @Router			/query/game/cql [post]
func queryCQL(ctx engine.Context, req *CQLQueryRequest) (*CQLQueryResponse, error) {
	cqlString := req.CQL

	// getComponentByName is a wrapper function that casts component.ComponentMetadata from ctx.GetComponentByName
	// to component.Component
	getComponentByName := func(name string) (component.Component, error) {
		comp, err := ctx.GetComponentByName(name)
		if err != nil {
			return nil, err
		}
		return comp, nil
	}

	resultFilter, err := cql.Parse(cqlString, getComponentByName)
	if err != nil {
		return nil, err
	}
	result := make([]cqlData, 0)
	var eachError error
	searchErr := NewSearch(ctx, resultFilter).Each(
		func(id entity.ID) bool {
			components, err := ctx.StoreReader().GetComponentTypesForEntity(id)
			if err != nil {
				eachError = err
				return false
			}
			resultElement := cqlData{
				ID:   id,
				Data: make([]json.RawMessage, 0),
			}

			for _, c := range components {
				data, err := ctx.StoreReader().GetComponentForEntityInRawJSON(c, id)
				if err != nil {
					eachError = err
					return false
				}
				resultElement.Data = append(resultElement.Data, data)
			}
			result = append(result, resultElement)
			return true
		},
	)
	if searchErr != nil {
		return nil, err
	}
	if eachError != nil {
		return nil, err
	}
	return &CQLQueryResponse{Results: result}, nil
}

type ListTxReceiptsRequest struct {
	StartTick uint64 `json:"startTick" mapstructure:"startTick"`
}

// ListTxReceiptsReply returns the transaction receipts for the given range of ticks. The interval is closed on
// StartTick and open on EndTick: i.e. [StartTick, EndTick)
// Meaning StartTick is included and EndTick is not. To iterate over all ticks in the future, use the returned
// EndTick as the StartTick in the next request. If StartTick == EndTick, the receipts list will be empty.
type ListTxReceiptsReply struct {
	StartTick uint64         `json:"startTick"`
	EndTick   uint64         `json:"endTick"`
	Receipts  []ReceiptEntry `json:"receipts"`
}

// ReceiptEntry represents a single transaction receipt. It contains an ID, a result, and a list of errors.
type ReceiptEntry struct {
	TxHash string  `json:"txHash"`
	Tick   uint64  `json:"tick"`
	Result any     `json:"result"`
	Errors []error `json:"errors"`
}

// receiptsQuery godoc
//
//	@Summary		Get transaction receipts from Cardinal
//	@Description	Get transaction receipts from Cardinal
//	@Accept			application/json
//	@Produce		application/json
//	@Param			ListTxReceiptsRequest	body		ListTxReceiptsRequest	true	"List Transaction Receipts Request"
//	@Success		200						{object}	ListTxReceiptsReply
//	@Failure		400						{string}	string	"Invalid transaction request"
//	@Router			/query/receipts/list [post]
func receiptsQuery(ctx engine.Context, req *ListTxReceiptsRequest) (*ListTxReceiptsReply, error) {
	reply := ListTxReceiptsReply{}
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
				Errors: r.Errs,
			})
		}
	}
	return &reply, nil
}
