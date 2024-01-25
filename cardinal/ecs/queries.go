package ecs

import (
	"encoding/json"
	"pkg.world.dev/world-engine/cardinal/ecs/cql"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

type DebugRequest struct{}

type debugStateElement struct {
	ID         entity.ID         `json:"id"`
	Components []json.RawMessage `json:"components"`
}

type DebugStateResponse []*debugStateElement

func queryDebugState(ctx EngineContext, _ *DebugRequest) (*DebugStateResponse, error) {
	result := make(DebugStateResponse, 0)
	search := NewSearch(filter.All())
	var eachClosureErr error
	searchEachErr := search.Each(
		ctx, func(id entity.ID) bool {
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
	Data []json.RawMessage `json:"data"`
}

type CQLQueryResponse struct {
	Results []cqlData `json:"results"`
}

func queryCQL(ctx EngineContext, req *CQLQueryRequest) (*CQLQueryResponse, error) {
	cqlString := req.CQL
	resultFilter, err := cql.Parse(cqlString, ctx.GetEngine().GetComponentByName)
	if err != nil {
		return nil, err
	}
	result := make([]cqlData, 0)
	var eachError error
	searchErr := NewSearch(resultFilter).Each(
		ctx, func(id entity.ID) bool {
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
	StartTick uint64    `json:"startTick"`
	EndTick   uint64    `json:"endTick"`
	Receipts  []Receipt `json:"receipts"`
}

// Receipt represents a single transaction receipt. It contains an ID, a result, and a list of errors.
type Receipt struct {
	TxHash string  `json:"txHash"`
	Tick   uint64  `json:"tick"`
	Result any     `json:"result"`
	Errors []error `json:"errors"`
}

func receiptsQuery(ctx EngineContext, req *ListTxReceiptsRequest) (*ListTxReceiptsReply, error) {
	eng := ctx.GetEngine()
	reply := ListTxReceiptsReply{}
	reply.EndTick = ctx.CurrentTick()
	size := eng.ReceiptHistorySize()
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
		currReceipts, err := eng.GetTransactionReceiptsForTick(t)
		if err != nil || len(currReceipts) == 0 {
			continue
		}
		for _, r := range currReceipts {
			reply.Receipts = append(reply.Receipts, Receipt{
				TxHash: string(r.TxHash),
				Tick:   t,
				Result: r.Result,
				Errors: r.Errs,
			})
		}
	}
	return &reply, nil
}
