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
	owner string
}

func UpdateEnergySystem(w World) {
	Energy.Each(w, func(entry *Entry) {
		energyPlanet := Energy.Get(entry)
		energyPlanet.Amt += 10 // bs whatever

	})
}

var (
	Energy  = NewComponentType[EnergyComponent]()
	Ownable = NewComponentType[OwnableComponent]()
)

func Test_ECS(t *testing.T) {

	world := NewWorld()

	// create a bunch of planets!
	world.CreateMany(100, Energy, Ownable)

	world.CreateMany(10, Energy)

	world.AddSystem(UpdateEnergySystem)
	world.Update()

	Energy.Each(world, func(entry *Entry) {
		energyPlanet := Energy.Get(entry)
		assert.Equal(t, int64(10), energyPlanet.Amt)
	})

	q := NewQuery(filter.Or(filter.Contains(Energy), filter.Contains(Ownable)))
	amt := q.Count(world)
	assert.Equal(t, 110, amt)
}
