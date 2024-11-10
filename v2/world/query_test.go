package world_test

import (
	"fmt"
	"testing"

	"github.com/goccy/go-json"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/v2"
	"pkg.world.dev/world-engine/cardinal/v2/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/v2/types"
	"pkg.world.dev/world-engine/cardinal/v2/world"
)

type Health struct {
	Value int
}

func (h Health) Name() string {
	return "health"
}

type QueryHealthRequest struct {
	Min int
}

type QueryHealthResponse struct {
	IDs []types.EntityID
}

func handleQueryHealth(wCtx world.WorldContextReadOnly, request *QueryHealthRequest) (*QueryHealthResponse, error) {
	resp := &QueryHealthResponse{}
	fmt.Println(request.Min)
	err := wCtx.Search(filter.Contains(Health{})).Each(
		func(id types.EntityID) bool {
			fmt.Println(id)
			health, err := world.GetComponent[Health](wCtx, id)
			if err != nil {
				return true
			}
			fmt.Println(health.Value)
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

func TestQueryExample(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterQuery[QueryHealthRequest, QueryHealthResponse](
		tf.World(),
		"query_health",
		handleQueryHealth,
	))

	assert.NilError(t, world.RegisterComponent[Health](tf.World()))

	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
		ids, err := world.CreateMany(wCtx, 100, Health{})
		if err != nil {
			return err
		}
		for i, id := range ids {
			assert.NilError(t, world.UpdateComponent[Health](wCtx, id, func(h *Health) *Health {
				h.Value = i
				return h
			}))
		}
		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	// Give each new entity health based on the ever-increasing index
	// No entities should have health over a million.
	respBz, err := tf.HandleQuery(world.DefaultGroup, "query_health", QueryHealthRequest{1_000_000})
	assert.NilError(t, err)

	var resp QueryHealthResponse
	err = json.Unmarshal(respBz, &resp)
	assert.NilError(t, err)
	assert.Equal(t, 0, len(resp.IDs))

	// All entities should have health over -100
	respBz, err = tf.HandleQuery(world.DefaultGroup, "query_health", QueryHealthRequest{-100})
	assert.NilError(t, err)

	err = json.Unmarshal(respBz, &resp)
	assert.NilError(t, err)
	assert.Equal(t, 100, len(resp.IDs))

	// Exactly 10 entities should have health at or above 90
	respBz, err = tf.HandleQuery(world.DefaultGroup, "query_health", QueryHealthRequest{90})
	assert.NilError(t, err)

	err = json.Unmarshal(respBz, &resp)
	assert.NilError(t, err)
	assert.Equal(t, 10, len(resp.IDs))
}

func TestQueryTypeNotStructs(t *testing.T) {
	str := "blah"
	err := world.RegisterQuery[string, string](
		cardinal.NewTestCardinal(t, nil).World(),
		"foo",
		func(world.WorldContextReadOnly, *string) (*string, error) {
			return &str, nil
		},
	)
	assert.ErrorContains(t, err, "the Request and Reply generics must be both structs")
}

func TestErrOnNoNameOrHandler(t *testing.T) {
	type foo struct{}
	testCases := []struct {
		name        string
		CreateQuery func() error
		shouldErr   bool
	}{
		{
			name: "error on no name",
			CreateQuery: func() error {
				return world.RegisterQuery[foo, foo](
					cardinal.NewTestCardinal(t, nil).World(),
					"",
					nil)
			},
			shouldErr: true,
		},
		{
			name: "error on no handler",
			CreateQuery: func() error {
				return world.RegisterQuery[foo, foo](
					cardinal.NewTestCardinal(t, nil).World(),
					"foo",
					nil)
			},
			shouldErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.shouldErr {
				err := tc.CreateQuery()
				assert.Assert(t, err != nil)
			} else {
				assert.NilError(t, tc.CreateQuery())
			}
		})
	}
}
