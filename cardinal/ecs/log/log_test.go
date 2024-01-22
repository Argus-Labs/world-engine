package log_test

import (
	"bytes"
	"context"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"strings"
	"testing"
	"time"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
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

func testSystem(wCtx cardinal.WorldContext) error {
	wCtx.Logger().Log().Msg("test")
	q := wCtx.NewSearch(filter.Contains(EnergyComp{}))
	err := q.Each(
		wCtx, func(entityId entity.ID) bool {
			energyPlanet, err := ecs.GetComponent[EnergyComp](wCtx, entityId)
			if err != nil {
				return false
			}
			energyPlanet.value += 10
			err = ecs.SetComponent[EnergyComp](wCtx, entityId, energyPlanet)
			return err == nil
		},
	)
	if err != nil {
		panic(err)
	}

	return nil
}

func testSystemWarningTrigger(wCtx cardinal.WorldContext) error {
	time.Sleep(time.Millisecond * 400)
	return testSystem(wCtx)
}

func TestWarningLogIfDuplicateSystemRegistered(t *testing.T) {
	engine := testutils.NewTestWorld(t).Engine()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	// replaces internal Logger with one that logs to the buf variable above.
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	engine.InjectLogger(&bufLogger)
	sysName := "foo"
	engine.RegisterSystemWithName(testSystem, sysName)
	engine.RegisterSystemWithName(testSystem, sysName)
	assert.Check(t, strings.Contains(buf.String(), "duplicate system registered: "+sysName))
}

func TestEngineLogger(t *testing.T) {
	engine := testutils.NewTestWorld(t).Engine()

	// testutils.NewTestWorld sets the log level to error, so we need to set it to zerolog.DebugLevel to pass this test
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	// replaces internal Logger with one that logs to the buf variable above.
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	engine.InjectLogger(&bufLogger)
	alphaTx := ecs.NewMessageType[SendEnergyTx, SendEnergyTxResult]("alpha")
	assert.NilError(t, engine.RegisterMessages(alphaTx))
	assert.NilError(t, ecs.RegisterComponent[EnergyComp](engine))
	log.Engine(&bufLogger, engine, zerolog.InfoLevel)
	jsonEngineInfoString := `{
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
	require.JSONEq(t, jsonEngineInfoString, buf.String())
	buf.Reset()
	energy, err := engine.GetComponentByName(EnergyComp{}.Name())
	assert.NilError(t, err)
	components := []component.ComponentMetadata{energy}
	wCtx := cardinal.NewWorldContext(engine)
	engine.RegisterSystem(testSystemWarningTrigger)
	err = engine.LoadGameState()
	assert.NilError(t, err)
	entityID, err := ecs.Create(wCtx, EnergyComp{})
	assert.NilError(t, err)
	logStrings := strings.Split(buf.String(), "\n")[:3]
	require.JSONEq(
		t, `
			{
				"level":"debug",
				"archetype_id":0,
				"message":"created"
			}`, logStrings[0],
	)
	require.JSONEq(
		t, `
			{
				"level":"debug",
				"components":[{
					"component_id":2,
					"component_name":"EnergyComp"
				}],
				"entity_id":0,"archetype_id":0
			}`, logStrings[1],
	)

	buf.Reset()

	// test log entity
	archetypeID, err := engine.StoreManager().GetArchIDForComponents(components)
	assert.NilError(t, err)
	log.Entity(&bufLogger, zerolog.DebugLevel, entityID, archetypeID, components)
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

	// create a system for logging.
	buf.Reset()
	ctx := context.Background()

	// testing output of logging a tick. Should log the system log and tick start and end strings.
	err = engine.Tick(ctx)
	assert.NilError(t, err)
	logStrings = strings.Split(buf.String(), "\n")[:4]
	// test tick start
	require.JSONEq(
		t, `
			{
				"level":"info",
				"tick":0,
				"message":"Tick started"
			}`, logStrings[0],
	)
	// test if updating component worked
	require.JSONEq(
		t, `
			{
				"level":"debug",
				"entity_id":"0",
				"component_name":"EnergyComp",
				"component_id":2,
				"message":"entity updated",
				"system":"log_test.testSystemWarningTrigger"
			}`, logStrings[2],
	)

	// testing log output for the creation of two entities.
	buf.Reset()
	_, err = ecs.CreateMany(wCtx, 2, EnergyComp{})
	assert.NilError(t, err)
	entityCreationStrings := strings.Split(buf.String(), "\n")[:2]
	require.JSONEq(
		t, `
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
			}`, entityCreationStrings[0],
	)
	require.JSONEq(
		t, `
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
			}`, entityCreationStrings[1],
	)
}
