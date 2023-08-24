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
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"strings"
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

func testSystem(w *ecs.World, _ *ecs.TransactionQueue, logger *ecs.Logger) error {
	logger.Log().Msg("test")
	energy.Each(w, func(entityId storage.EntityID) bool {
		energyPlanet, err := energy.Get(w, entityId)
		if err != nil {
			return false
		}
		energyPlanet.value += 10 // bs whatever
		err = energy.Set(w, entityId, energyPlanet)
		if err != nil {
			return false
		}
		return true
	})

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
	buf.Reset()
	archetypeId := w.GetArchetypeForComponents([]component.IComponentType{energy})
	archetype_creations_json_string := buf.String()
	require.JSONEq(t, `
			{
				"level":"debug",
				"archetype_id":0,
				"message":"created"
			}`, archetype_creations_json_string)
	entityId, err := w.Create(w.Archetype(archetypeId).Layout().Components()...)
	assert.NilError(t, err)
	buf.Reset()

	// test log entity
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

	//create a system for logging.
	buf.Reset()
	w.AddSystems(testSystem)
	err = w.LoadGameState()
	assert.NilError(t, err)
	ctx := context.Background()

	// testing output of logging a tick. Should log the system log and tick start and end strings.
	err = w.Tick(ctx)
	assert.NilError(t, err)
	logString := buf.String()
	logStrings := strings.Split(logString, "\n")[:4]
	// test tick start
	require.JSONEq(t, `
			{
				"level":"info",
				"tick":"0",
				"message":"Tick started"
			}`, logStrings[0])
	// test if system name recorded in log
	require.JSONEq(t, `
			{
				"system":"ecs_test.testSystem",
				"message":"test"
			}`, logStrings[1])
	// test if updating component worked
	require.JSONEq(t, `
			{
				"level":"debug",
				"entity_id":"0",
				"component_name":"EnergyComp",
				"component_id":2,
				"message":"entity updated"
			}`, logStrings[2])
	// test tick end
	require.JSONEq(t, `
				{
					"level":"info",
					"tick":"0",
					"processed_transactions":0,
					"message":"Tick ended"
				}`, logStrings[3])

	// testing log output for the creation of two entities.
	buf.Reset()
	_, err = w.CreateMany(2, []component.IComponentType{energy}...)
	assert.NilError(t, err)
	entityCreationStrings := strings.Split(buf.String(), "\n")[:2]
	require.JSONEq(t, `
			{
				"level":"debug",
				"components":
					[
						{
							"component_id":2,
							"component_name":"EnergyComp"
						}
					],
				"entity_id":1,
				"archetype_id":0
			}`, entityCreationStrings[0])
	require.JSONEq(t, `
			{
				"level":"debug",
				"components":
					[
						{
							"component_id":2,
							"component_name":"EnergyComp"
						}
					],
				"entity_id":2,
				"archetype_id":0
			}`, entityCreationStrings[1])
}
