package ecs_test

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"testing"

	"github.com/rs/zerolog"
	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
)

type SendEnergyTx struct {
	From, To string
	Amount   uint64
}

type SendEnergyTxResult struct{}

type EnergyComp struct {
	value int
}

var energy = ecs.NewComponentType[EnergyComp]()

func TestWorldLogger(t *testing.T) {

	traceId := "test_trace_id"
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	//opts is used to hijack the Logger placed in the world object and replace it with something that writes to buf(above)
	//so it's more easily unit tested. All logging methods called by w.Logger are now written to the buf variable.
	opt := func(w *ecs.World) {
		w.Logger = ecs.NewWorldLogger(&bufLogger, w)
	}
	w := inmem.NewECSWorldForTest(t, opt)

	alphaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("alpha")
	assert.NilError(t, w.RegisterTransactions(alphaTx))
	err := w.RegisterComponents(energy)
	assert.NilError(t, err)
	//Test log world state
	w.Logger.LogWorldState(traceId, zerolog.InfoLevel, "test message")
	jsonWorldInfoString := `{
					"trace_id":"test_trace_id",
					"level":"info",
					"trace_id":"test_trace_id",
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
	err = w.Logger.LogEntity(traceId, zerolog.DebugLevel, entityId, "test message")
	assert.NilError(t, err)
	jsonEntityInfoString := `
		{
			"trace_id":"test_trace_id",
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

}
