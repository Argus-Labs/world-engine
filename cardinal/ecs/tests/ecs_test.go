package tests

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/argus-labs/world-engine/cardinal/ecs"

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

func newWorldForTest(t *testing.T) ecs.World {
	s := miniredis.RunT(t)
	rs := storage.NewRedisStorage(storage.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, "0")
	worldStorage := storage.NewWorldStorage(
		storage.Components{Store: &rs, ComponentIndices: &rs}, &rs, storage.NewArchetypeComponentIndex(), storage.NewArchetypeAccessor(), &rs, &rs)

	return ecs.NewWorld(worldStorage)
}

func TestECS(t *testing.T) {
	world := newWorldForTest(t)

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

func TestVelocitySimulation(t *testing.T) {
	world := newWorldForTest(t)
	type Pos struct {
		X, Y float64
	}
	type Vel struct {
		DX, DY float64
	}
	Position := ecs.NewComponentType[Pos]()
	Velocity := ecs.NewComponentType[Vel]()

	shipEntity, err := world.Create(Position, Velocity)
	assert.NilError(t, err)
	shipEntry, err := world.Entry(shipEntity)
	assert.NilError(t, err)
	Position.Set(shipEntry, &Pos{1, 2})
	Velocity.Set(shipEntry, &Vel{3, 4})
	wantPos := Pos{4, 6}

	Velocity.Each(world, func(e *storage.Entry) {
		vel, err := Velocity.Get(e)
		assert.NilError(t, err)
		pos, err := Position.Get(e)
		assert.NilError(t, err)
		newPos := &Pos{pos.X + vel.DX, pos.Y + vel.DY}
		Position.Set(e, newPos)
	})

	finalPos, err := Position.Get(shipEntry)
	assert.NilError(t, err)
	assert.Equal(t, wantPos, finalPos)
}

func TestCanSetDefaultValue(t *testing.T) {
	world := newWorldForTest(t)
	type Owner struct {
		Name string
	}
	wantOwner := Owner{"Jeff"}
	owner := ecs.NewComponentType[Owner](ecs.WithDefault(wantOwner))

	alpha, err := world.Create(owner)
	assert.NilError(t, err)

	alphaEntry, err := world.Entry(alpha)
	assert.NilError(t, err)

	alphaOwner, err := owner.Get(alphaEntry)
	assert.NilError(t, err)
	assert.Equal(t, alphaOwner, wantOwner)

	alphaOwner.Name = "Bob"
	owner.Set(alphaEntry, &alphaOwner)

	newOwner, err := owner.Get(alphaEntry)
	assert.Equal(t, newOwner.Name, "Bob")
}

func TestCanRemoveEntity(t *testing.T) {
	world := newWorldForTest(t)
	type Tuple struct {
		A, B int
	}

	tuple := ecs.NewComponentType[Tuple]()
	entities, err := world.CreateMany(2, tuple)
	assert.NilError(t, err)

	// Make sure we find exactly 2 entries
	count := 0
	tuple.Each(world, func(entry *storage.Entry) {
		_, err := tuple.Get(entry)
		assert.NilError(t, err)
		count++
	})

	assert.Equal(t, count, 2)
	err = world.Remove(entities[0])
	assert.NilError(t, err)

	// Now we should only find 1 entry
	count = 0
	tuple.Each(world, func(entry *storage.Entry) {
		_, err := tuple.Get(entry)
		assert.NilError(t, err)
		count++
	})
	assert.Equal(t, count, 1)

	// This entity was Removed, so we shouldn't be able to find it
	_, err = world.Entry(entities[0])
	assert.Check(t, err != nil)

	// Remove the other entity
	err = world.Remove(entities[1])
	assert.NilError(t, err)
	count = 0
	tuple.Each(world, func(entry *storage.Entry) {
		_, err := tuple.Get(entry)
		assert.NilError(t, err)
		count++
	})
	assert.Equal(t, count, 0)

	// This entity was Removed, so we shouldn't be able to find it
	_, err = world.Entry(entities[0])
	assert.Check(t, err != nil)
}
