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

type SendEnergyTx struct {
	From, To string
	Amount   uint64
}

type SendEnergyTxResult struct{}

type EnergyComp struct {
	value int
}

var energy = ecs.NewComponentType[EnergyComp]()

func testSystem(_ *ecs.World, _ *ecs.TransactionQueue, logger *ecs.Logger) error {
	logger.Log().Msg("test")
	return nil
}

func TestWorldLogger(t *testing.T) {

	w := inmem.NewECSWorldForTest(t)
	//replaces internal logger with one that logs to the buf variable above.
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	cardinalLogger := ecs.Logger{
		&bufLogger,
	}
	w.InjectLogger(&cardinalLogger)
	alphaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("alpha")
	assert.NilError(t, w.RegisterTransactions(alphaTx))
	err := w.RegisterComponents(energy)
	assert.NilError(t, err)
	cardinalLogger.LogWorld(w, zerolog.InfoLevel)
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
					"total_systems":2,
					"systems":
						[
							"ecs.RegisterPersonaSystem",
							"ecs.AuthorizePersonaAddressSystem"
						]
				}
`
	//require.JSONEq compares json strings for equality.
	require.JSONEq(t, buf.String(), jsonWorldInfoString)
	archetypeId := w.GetArchetypeForComponents([]component.IComponentType{energy})
	entityId, err := w.Create(w.Archetype(archetypeId).Layout().Components()...)
	assert.NilError(t, err)
	buf.Reset()
	//test log entity
	err = cardinalLogger.LogEntity(w, zerolog.DebugLevel, entityId)
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
			"archetype_id":0
		}`
	require.JSONEq(t, buf.String(), jsonEntityInfoString)
	buf.Reset()
	w.AddSystems(testSystem)
	err = w.LoadGameState()
	assert.NilError(t, err)
	ctx := context.Background()
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
