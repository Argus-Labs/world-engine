package cardinal_test

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/cardinaltestutils"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

type QueryHealthRequest struct {
	Min int
}

type QueryHealthResponse struct {
	IDs []cardinal.EntityID
}

func handleQueryHealth(
	worldCtx cardinal.WorldContext,
	request *QueryHealthRequest,
) (*QueryHealthResponse, error) {
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

func TestNewQueryTypeWithEVMSupport(t *testing.T) {
	// This test just makes sure that NeQueryTypeWithEVMSupport maintains api compatibility.
	// it is mainly here to check for compiler errors.
	type FooReq struct {
		X uint64
	}
	type FooReply struct {
		Y uint64
	}
	_ = cardinal.RegisterQueryWithEVMSupport[FooReq, FooReply](
		cardinaltestutils.NewTestWorld(t),
		"query_health",
		func(
			_ cardinal.WorldContext,
			_ *FooReq) (*FooReply, error) {
			return &FooReply{}, errors.New("this function should never get called")
		})
}

func TestQueryExample(t *testing.T) {
	world, _ := cardinaltestutils.MakeWorldAndTicker(t)
	testutils.AssertNilErrorWithTrace(t, cardinal.RegisterComponent[Health](world))
	testutils.AssertNilErrorWithTrace(
		t,
		cardinal.RegisterQuery[QueryHealthRequest, QueryHealthResponse](
			world,
			"query_health",
			handleQueryHealth,
		),
	)

	worldCtx := cardinaltestutils.WorldToWorldContext(world)
	ids, err := cardinal.CreateMany(worldCtx, 100, Health{})
	testutils.AssertNilErrorWithTrace(t, err)
	// Give each new entity health based on the ever-increasing index
	for i, id := range ids {
		testutils.AssertNilErrorWithTrace(t, cardinal.UpdateComponent[Health](worldCtx, id, func(h *Health) *Health {
			h.Value = i
			return h
		}))
	}

	// No entities should have health over a million.
	q, err := world.Instance().GetQueryByName("query_health")
	testutils.AssertNilErrorWithTrace(t, err)

	resp, err := q.HandleQuery(worldCtx.Instance(), QueryHealthRequest{1_000_000})
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 0, len(resp.(*QueryHealthResponse).IDs))

	// All entities should have health over -100
	resp, err = q.HandleQuery(worldCtx.Instance(), QueryHealthRequest{-100})
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 100, len(resp.(*QueryHealthResponse).IDs))

	// Exactly 10 entities should have health at or above 90
	resp, err = q.HandleQuery(worldCtx.Instance(), QueryHealthRequest{90})
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 10, len(resp.(*QueryHealthResponse).IDs))
}
