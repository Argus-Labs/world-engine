package ecs_test

import (
	"context"
	"encoding/json"
	"errors"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/sign"
	"testing"
)

type fooComp struct {
	X string
	Y int
}

func (fooComp) Name() string { return "foo" }

type barComp struct {
	Z bool
	R uint64
}

func (barComp) Name() string { return "bar" }

func TestDebugStateQuery(t *testing.T) {
	eng := testutils.NewTestFixture(t, nil).Engine
	ecs.MustRegisterComponent[barComp](eng)
	ecs.MustRegisterComponent[fooComp](eng)

	type barFooEntity struct {
		barComp
		fooComp
	}

	entities := make([]barFooEntity, 0)
	entities = append(entities,
		barFooEntity{
			barComp{true, 320},
			fooComp{"lol", 39},
		},
		barFooEntity{
			barComp{false, 3209352835},
			fooComp{"omg", -23},
		},
	)

	eng.RegisterSystem(func(ctx ecs.EngineContext) error {
		for _, entity := range entities {
			_, err := ecs.Create(ctx, entity.barComp, entity.fooComp)
			assert.NilError(t, err)
		}
		return nil
	})
	assert.NilError(t, eng.LoadGameState())
	assert.NilError(t, eng.Tick(context.Background()))

	qry, err := eng.GetQueryByName("state")
	assert.NilError(t, err)

	res, err := qry.HandleQuery(ecs.NewReadOnlyEngineContext(eng), ecs.DebugRequest{})
	assert.NilError(t, err)

	results := *res.(*ecs.DebugStateResponse)

	bar, err := eng.GetComponentByName(barComp{}.Name())
	assert.NilError(t, err)

	foo, err := eng.GetComponentByName(fooComp{}.Name())
	assert.NilError(t, err)

	assert.Len(t, results, 2)
	for i, result := range results {
		barData, err := bar.Decode(result.Components[0])
		assert.NilError(t, err)
		fooData, err := foo.Decode(result.Components[1])
		assert.NilError(t, err)

		assert.Equal(t, barData.(barComp), entities[i].barComp)
		assert.Equal(t, fooData.(fooComp), entities[i].fooComp)
	}
}

func TestCQLQuery(t *testing.T) {
	eng := testutils.NewTestFixture(t, nil).Engine
	ecs.MustRegisterComponent[barComp](eng)
	ecs.MustRegisterComponent[fooComp](eng)

	firstBar := barComp{true, 420}
	secondBar := barComp{false, 20}
	bars := []barComp{firstBar, secondBar}
	eng.RegisterSystem(func(ctx ecs.EngineContext) error {
		_, err := ecs.Create(ctx, firstBar, fooComp{"hi", 32})
		assert.NilError(t, err)
		_, err = ecs.Create(ctx, secondBar)
		assert.NilError(t, err)
		_, err = ecs.Create(ctx, fooComp{"no", 33})
		assert.NilError(t, err)
		return nil
	})
	assert.NilError(t, eng.LoadGameState())
	assert.NilError(t, eng.Tick(context.Background()))

	barComponent, err := eng.GetComponentByName(barComp{}.Name())
	assert.NilError(t, err)

	query, err := eng.GetQueryByName("cql")
	assert.NilError(t, err)

	res, err := query.HandleQuery(ecs.NewReadOnlyEngineContext(eng), ecs.CQLQueryRequest{CQL: "CONTAINS(bar)"})
	assert.NilError(t, err)
	result, ok := res.(*ecs.CQLQueryResponse)
	assert.True(t, ok)

	assert.Len(t, result.Results, 2)

	for i, r := range result.Results {
		gotBarAny, err := barComponent.Decode(r.Data[0])
		assert.NilError(t, err)
		gotBar, ok := gotBarAny.(barComp)
		assert.True(t, ok)
		assert.Equal(t, gotBar, bars[i])
	}
}

func TestCQLQueryErrorOnBadFormat(t *testing.T) {
	eng := testutils.NewTestFixture(t, nil).Engine
	assert.NilError(t, eng.LoadGameState())
	query, err := eng.GetQueryByName("cql")
	assert.NilError(t, err)
	res, err := query.HandleQuery(ecs.NewReadOnlyEngineContext(eng), ecs.CQLQueryRequest{CQL: "MEOW(FOO)"})
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "failed to parse CQL string")
}

func TestCQLQueryNonExistentComponent(t *testing.T) {
	eng := testutils.NewTestFixture(t, nil).Engine
	assert.NilError(t, eng.LoadGameState())
	query, err := eng.GetQueryByName("cql")
	assert.NilError(t, err)
	res, err := query.HandleQuery(ecs.NewReadOnlyEngineContext(eng), ecs.CQLQueryRequest{CQL: "CONTAINS(meow)"})
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), `component "meow" must be registered before being used`)
}

func TestReceiptsQuery(t *testing.T) {
	eng := testutils.NewTestFixture(t, nil).Engine
	type fooIn struct{}
	type fooOut struct{ Y int }
	fooMsg := ecs.NewMessageType[fooIn, fooOut]("foo")
	err := eng.RegisterMessages(fooMsg)
	assert.NilError(t, err)
	eng.RegisterSystem(func(ctx ecs.EngineContext) error {
		fooMsg.Each(ctx, func(t ecs.TxData[fooIn]) (fooOut, error) {
			if ctx.CurrentTick()%2 == 0 {
				return fooOut{Y: 4}, nil
			}

			return fooOut{}, errors.New("omg")
		})
		return nil
	})
	assert.NilError(t, eng.LoadGameState())
	txHash1 := fooMsg.AddToQueue(eng, fooIn{}, &sign.Transaction{PersonaTag: "ty"})
	assert.NilError(t, eng.Tick(context.Background()))
	txHash2 := fooMsg.AddToQueue(eng, fooIn{}, &sign.Transaction{PersonaTag: "ty"})
	assert.NilError(t, eng.Tick(context.Background()))

	qry, err := eng.GetQueryByName("list")
	assert.NilError(t, err)

	res, err := qry.HandleQuery(ecs.NewReadOnlyEngineContext(eng), &ecs.ListTxReceiptsRequest{})
	assert.NilError(t, err)
	reply, ok := res.(*ecs.ListTxReceiptsReply)
	assert.True(t, ok)

	assert.Equal(t, reply.StartTick, uint64(0))
	assert.Equal(t, reply.EndTick, eng.CurrentTick())
	assert.Len(t, reply.Receipts, 2)

	expectedReceipt1 := ecs.Receipt{
		TxHash: string(txHash1),
		Tick:   0,
		Result: fooOut{Y: 4},
		Errors: nil,
	}
	expectedJSON1, err := json.Marshal(expectedReceipt1)
	assert.NilError(t, err)
	expectedReceipt2 := ecs.Receipt{
		TxHash: string(txHash2),
		Tick:   1,
		Result: nil,
		Errors: []error{errors.New("omg")},
	}
	expectedJSON2, err := json.Marshal(expectedReceipt2)
	assert.NilError(t, err)

	// comparing via json since internally, eris is involved, and makes it a bit harder to compare.
	json1, err := json.Marshal(reply.Receipts[0])
	assert.NilError(t, err)
	json2, err := json.Marshal(reply.Receipts[1])
	assert.NilError(t, err)

	assert.Equal(t, string(expectedJSON1), string(json1))
	assert.Equal(t, string(expectedJSON2), string(json2))
}
