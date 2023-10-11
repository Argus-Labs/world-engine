package log_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/log"
)

type SendEnergyTx struct {
	From, To string
	Amount   uint64
}

type SendEnergyTxResult struct{}

type EnergyComp struct {
	value int
}

func (EnergyComp) Name() string {
	return "EnergyComp"
}

var energy = ecs.NewComponentType[EnergyComp]("EnergyComp")

func testSystem(w *ecs.World, _ *transaction.TxQueue, logger *log.Logger) error {
	logger.Log().Msg("test")
	energy.Each(w, func(entityId entity.ID) bool {
		energyPlanet, err := ecs.GetComponent[EnergyComp](w, entityId)
		//energyPlanet, err := energy.Get(w, entityId)
		if err != nil {
			return false
		}
		energyPlanet.value += 10 // bs whatever
		err = ecs.SetComponent[EnergyComp](w, entityId, energyPlanet)
		//err = energy.Set(w, entityId, energyPlanet)
		if err != nil {
			return false
		}
		return true
	})

	return nil
}

func testSystemWarningTrigger(w *ecs.World, tx *transaction.TxQueue, logger *log.Logger) error {
	time.Sleep(time.Millisecond * 400)
	return testSystem(w, tx, logger)
}

func TestWorldLogger(t *testing.T) {

	w := ecs.NewTestWorld(t)
	//replaces internal Logger with one that logs to the buf variable above.
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	cardinalLogger := log.Logger{
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
	archetypeId, err := w.StoreManager().GetArchIDForComponents([]component.IComponentType{energy})
	assert.NilError(t, err)
	archetype_creations_json_string := buf.String()
	require.JSONEq(t, `
			{
				"level":"debug",
				"archetype_id":0,
				"message":"created"
			}`, archetype_creations_json_string)
	components := w.StoreManager().GetComponentTypesForArchID(archetypeId)
	entityId, err := ecs.Create(w, EnergyComp{})
	assert.NilError(t, err)
	buf.Reset()

	entity, err := w.StoreManager().GetEntity(entityId)
	assert.NilError(t, err)

	// test log entity
	cardinalLogger.LogEntity(zerolog.DebugLevel, entity, components)
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
	w.AddSystems(testSystemWarningTrigger)
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
				"system":"log_test.testSystemWarningTrigger",
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
	buf.Reset()
	sanitizeJsonBytes := func(json []byte) []byte {
		json = bytes.Replace(json, []byte("\t"), []byte(""), -1)
		json = bytes.Replace(json, []byte("\n"), []byte(""), -1)
		json = bytes.Replace(json, []byte("\r"), []byte(""), -1)
		return json
	}
	var map1, map2 map[string]interface{}
	//tick execution time is not tested.
	json1 := []byte(`{
				 "level":"warn",
				 "tick":"0",
				 "tick_execution_time": 0, 
				 "message":"tick ended, (warning: tick exceeded 100ms)"
			 }`)
	json1 = sanitizeJsonBytes(json1)
	if err = json.Unmarshal(json1, &map1); err != nil {
		t.Fatalf("Error unmarshalling json1: %v", err)
	}
	if err = json.Unmarshal([]byte(logStrings[3]), &map2); err != nil {
		t.Fatalf("Error unmarshalling buf: %v", err)
	}
	names := []string{"level", "tick", "tick_execution_time", "message"}
	for _, name := range names {
		v1, ok := map1[name]
		if !ok {
			t.Errorf("Should be a value in %s", name)
		}
		v2, ok := map2[name]
		if !ok {
			t.Errorf("Should be a value in %s", name)
		}
		if name == "tick_execution_time" {
			//time is not deterministic in the context of unit tests, therefore it is not unit testable.
		} else {
			assert.Equal(t, v1, v2)
		}
	}

	// testing log output for the creation of two entities.
	buf.Reset()
	_, err = ecs.CreateMany(w, 2, EnergyComp{})
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
