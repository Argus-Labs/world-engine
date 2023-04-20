package tests

import (
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

type EnergyComponent struct {
	Amt int64
	Cap int64
}

type OwnableComponent struct {
	Owner string
}

func UpdateEnergySystem(w ecs.World) {
	Energy.Each(w, func(entry *storage.Entry) {
		energyPlanet, err := Energy.Get(entry)
		if err != nil {
			panic(err)
		}
		energyPlanet.Amt += 10 // bs whatever
		err = Energy.Set(entry, &energyPlanet)
		if err != nil {
			panic(err)
		}
	})
}

var (
	Energy  = ecs.NewComponentType[EnergyComponent]()
	Ownable = ecs.NewComponentType[OwnableComponent]()
)

func Test_ECS(t *testing.T) {

	redisClient := getRedisClient(t)
	world := ecs.NewWorld(storage.NewRedisStorage(redisClient, "0"))

	world.RegisterComponents(Energy, Ownable)

	// create a bunch of planets!
	numPlanets := 5
	_, err := world.CreateMany(numPlanets, Energy, Ownable)
	assert.NilError(t, err)

	numEnergyOnly := 10
	_, err = world.CreateMany(numEnergyOnly, Energy)
	assert.NilError(t, err)

	world.AddSystem(UpdateEnergySystem)
	world.Update()

	Energy.Each(world, func(entry *storage.Entry) {
		energyPlanet, err := Energy.Get(entry)
		assert.NilError(t, err)
		assert.Equal(t, int64(10), energyPlanet.Amt)
	})

	q := ecs.NewQuery(filter.Or(filter.Contains(Energy), filter.Contains(Ownable)))
	amt := q.Count(world)
	assert.Equal(t, numPlanets+numEnergyOnly, amt)
}
