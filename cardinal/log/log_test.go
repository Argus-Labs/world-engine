package log_test

import (
	"bytes"
	"context"
	"fmt"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/log"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"strings"
	"testing"
	"time"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

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

func testSystem(eCtx engine.Context) error {
	eCtx.Logger().Log().Msg("test")
	q := cardinal.NewSearch(eCtx, filter.Contains(EnergyComp{}))
	err := q.Each(
		func(entityId entity.ID) bool {
			energyPlanet, err := cardinal.GetComponent[EnergyComp](eCtx, entityId)
			if err != nil {
				return false
			}
			energyPlanet.value += 10
			err = cardinal.SetComponent[EnergyComp](eCtx, entityId, energyPlanet)
			return err == nil
		},
	)
	if err != nil {
		panic(err)
	}

	return nil
}

func testSystemWarningTrigger(eCtx engine.Context) error {
	time.Sleep(time.Millisecond * 400)
	return testSystem(eCtx)
}

func TestEngineLogger(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World

	// Ensure logs are enabled
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	// replaces internal Logger with one that logs to the buf variable above.
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	world.InjectLogger(&bufLogger)
	alphaTx := cardinal.NewMessageType[SendEnergyTx, SendEnergyTxResult]("alpha")
	assert.NilError(t, cardinal.RegisterMessages(world, alphaTx))
	assert.NilError(t, cardinal.RegisterComponent[EnergyComp](world))
	log.World(&bufLogger, world, zerolog.InfoLevel)
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
							"cardinal.RegisterPersonaSystem",
							"cardinal.AuthorizePersonaAddressSystem"
						]
				}
`
	require.JSONEq(t, jsonEngineInfoString, buf.String())
	buf.Reset()
	energy, err := world.GetComponentByName(EnergyComp{}.Name())
	assert.NilError(t, err)
	components := []component.ComponentMetadata{energy}
	eCtx := cardinal.NewWorldContext(world)
	err = cardinal.RegisterSystems(world, testSystemWarningTrigger)
	assert.NilError(t, err)
	err = world.LoadGameState()
	assert.NilError(t, err)
	entityID, err := cardinal.Create(eCtx, EnergyComp{})
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
	archetypeID, err := world.GameStateManager().GetArchIDForComponents(components)
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

	// Create a system for logging.
	buf.Reset()
	ctx := context.Background()

	// testing output of logging a tick. Should log the system log and tick start and end strings.
	err = world.Tick(ctx)
	assert.NilError(t, err)
	logStrings = strings.Split(buf.String(), "\n")[:3]
	fmt.Println(logStrings)
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
	_, err = cardinal.CreateMany(eCtx, 2, EnergyComp{})
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
