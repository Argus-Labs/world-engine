package cardinal_test

import (
	"errors"
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
)

type QueryHealthRequest struct {
	Min int
}

type QueryHealthResponse struct {
	IDs []cardinal.EntityID
}

func handleQueryHealth(worldCtx cardinal.WorldContext, request *QueryHealthRequest) (*QueryHealthResponse, error) {
	q, err := worldCtx.NewSearch(cardinal.Exact(Health{}))
	if err != nil {
		return nil, err
	}
	resp := &QueryHealthResponse{}
	err = q.Each(worldCtx, func(id cardinal.EntityID) bool {
		var health *Health
		health, err = cardinal.GetComponent[Health](worldCtx, id)
		if err != nil {
			return true
		}
		if health.Value < request.Min {
			return true
		}
		resp.IDs = append(resp.IDs, id)
		return true
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func TestNewQueryTypeWithEVMSupport(_ *testing.T) {
	// This test just makes sure that NeQueryTypeWithEVMSupport maintains api compatibility.
	// it is mainly here to check for compiler errors.
	type FooReq struct {
		X uint64
	}
	type FooReply struct {
		Y uint64
	}
	cardinal.NewQueryTypeWithEVMSupport[FooReq, FooReply](
		"query_health",
		func(
			_ cardinal.WorldContext,
			_ FooReq) (FooReply, error) {
			return FooReply{}, errors.New("this function should never get called")
		})
}

var queryHealth = cardinal.NewQueryType[*QueryHealthRequest, *QueryHealthResponse]("query_health", handleQueryHealth)

func TestQueryExample(t *testing.T) {
	world, _ := testutils.MakeWorldAndTicker(t)
	assert.NilError(t, cardinal.RegisterComponent[Health](world))
	assert.NilError(t, cardinal.RegisterQueries(world, queryHealth))

	worldCtx := testutils.WorldToWorldContext(world)
	ids, err := cardinal.CreateMany(worldCtx, 100, Health{})
	assert.NilError(t, err)
	// Give each new entity health based on the ever-increasing index
	for i, id := range ids {
		assert.NilError(t, cardinal.UpdateComponent[Health](worldCtx, id, func(h *Health) *Health {
			h.Value = i
			return h
		}))
	}

	// No entities should have health over a million.
	resp, err := queryHealth.DoQuery(worldCtx, &QueryHealthRequest{1_000_000})
	assert.NilError(t, err)
	assert.Equal(t, 0, len(resp.IDs))

	// All entities should have health over -100
	resp, err = queryHealth.DoQuery(worldCtx, &QueryHealthRequest{-100})
	assert.NilError(t, err)
	assert.Equal(t, 100, len(resp.IDs))

	// Exactly 10 entities should have health at or above 90
	resp, err = queryHealth.DoQuery(worldCtx, &QueryHealthRequest{90})
	assert.NilError(t, err)
	assert.Equal(t, 10, len(resp.IDs))
}
