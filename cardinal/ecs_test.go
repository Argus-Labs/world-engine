package cardinal

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/argus-labs/cardinal/filter"
)

type EnergyComponent struct {
	Amt int64
	Cap int64
}

type OwnableComponent struct {
	Owner string
}

func UpdateEnergySystem(w World) {
	Energy.Each(w, func(entry *Entry) {
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
	Energy  = NewComponentType[EnergyComponent]()
	Ownable = NewComponentType[OwnableComponent]()
)

func Test_ECS(t *testing.T) {

	world := NewWorld()

	// create a bunch of planets!
	_, err := world.CreateMany(100, Energy, Ownable)
	assert.NilError(t, err)

	_, err = world.CreateMany(10, Energy)
	assert.NilError(t, err)

	world.AddSystem(UpdateEnergySystem)
	world.Update()

	Energy.Each(world, func(entry *Entry) {
		energyPlanet, err := Energy.Get(entry)
		assert.NilError(t, err)
		assert.Equal(t, int64(10), energyPlanet.Amt)
	})

	q := NewQuery(filter.Or(filter.Contains(Energy), filter.Contains(Ownable)))
	amt := q.Count(world)
	assert.Equal(t, 110, amt)
}
