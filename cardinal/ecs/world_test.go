package ecs_test

import (
	"bytes"
	"context"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
	"testing"
)

func TestAddSystems(t *testing.T) {
	count := 0
	sys := func(w *ecs.World, txq *ecs.TransactionQueue, _ *zerolog.Logger) error {
		count++
		return nil
	}

	w := inmem.NewECSWorldForTest(t)
	w.AddSystems(sys, sys, sys)
	err := w.LoadGameState()
	assert.NilError(t, err)

	err = w.Tick(context.Background())
	assert.NilError(t, err)

	assert.Equal(t, count, 3)
}

func TestSetNamespace(t *testing.T) {
	id := "foo"
	w := inmem.NewECSWorldForTest(t, ecs.WithNamespace(id))
	assert.Equal(t, w.Namespace(), id)
}

func testSystem(world *ecs.World, queue *ecs.TransactionQueue, logger *zerolog.Logger) error {
	logger.Log().Msg("test")
	return nil
}

func TestWorldLogger(t *testing.T) {
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)

	w := inmem.NewECSWorldForTest(t)
	//replaces internal logger with one that logs to the buf variable above.
	w.Logger = &bufLogger

	alphaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("alpha")
	assert.NilError(t, w.RegisterTransactions(alphaTx))
	err := w.RegisterComponents(energy)
	assert.NilError(t, err)
	//Test log world state
	//traceLogger := w.CreateTraceLogger(traceId)
	w.LogWorldState(zerolog.InfoLevel, "test message")
	jsonWorldInfoString := `{
					"level":"info",
					"total_components":2,
					"components":
						[
							{
								"component_id":1,
								"component_name":"SignerComponent"
							},
							{
								"component_id":2,
								"component_name":"EnergyComp"
							}
						],
					"total_systems":1,
					"systems":
						[
							"ecs.RegisterPersonaSystem"
						],
					"message":"test message"
				}
`
	//require.JSONEq compares json strings for equality.
	require.JSONEq(t, buf.String(), jsonWorldInfoString)
	archetypeId := w.GetArchetypeForComponents([]component.IComponentType{energy})
	entityId, err := w.Create(w.Archetype(archetypeId).Layout().Components()...)
	assert.NilError(t, err)
	buf.Reset()
	//test log entity
	err = w.LogEntity(zerolog.DebugLevel, entityId, "test message")
	assert.NilError(t, err)
	jsonEntityInfoString := `
		{
			"level":"debug",
			"components":[
				{
					"component_id":2,
					"component_name":"EnergyComp"
				}],
			"entity_id":0,
			"archetype_id":0,
			"message":"test message"
		}`

	require.JSONEq(t, buf.String(), jsonEntityInfoString)
	buf.Reset()
	w.AddSystems(testSystem)
	ctx := context.Background()
	err = w.LoadGameState()
	assert.NilError(t, err)
	err = w.Tick(ctx)
	assert.NilError(t, err)
	jsonSystemLogTestString := `
		{
    		"system": "ecs_test.testSystem",
    		"message": "test"
		}
	`
	require.JSONEq(t, buf.String(), jsonSystemLogTestString)
}
