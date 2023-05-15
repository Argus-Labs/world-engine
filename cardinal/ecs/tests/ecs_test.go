package tests

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"gotest.tools/v3/assert"
)

type EnergyComponent struct {
	Amt int64
	Cap int64
}

type OwnableComponent struct {
	Owner string
}

func UpdateEnergySystem(w *ecs.World) {
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

func newWorldForTest(t *testing.T) *ecs.World {
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
	world.RegisterComponents(Position, Velocity)

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
	world.RegisterComponents(owner)

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
	world.RegisterComponents(tuple)

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

func TestCanRemoveEntriesDuringCallToEach(t *testing.T) {
	world := newWorldForTest(t)
	type CountComponent struct {
		Val int
	}
	Count := ecs.NewComponentType[CountComponent]()
	world.RegisterComponents(Count)

	_, err := world.CreateMany(10, Count)
	assert.NilError(t, err)

	// Remove the even entries
	itr := 0
	Count.Each(world, func(entry *storage.Entry) {
		if itr%2 == 0 {
			assert.NilError(t, world.Remove(entry.Ent))
		} else {
			assert.NilError(t, Count.Set(entry, &CountComponent{itr}))
		}
		itr++
	})
	// Verify we did this Each the correct number of times
	assert.Equal(t, 10, itr)

	seen := map[int]int{}
	Count.Each(world, func(entry *storage.Entry) {
		c, err := Count.Get(entry)
		assert.NilError(t, err)
		seen[c.Val]++
	})

	// Verify we're left with exactly 5 odd values between 1 and 9
	assert.Equal(t, len(seen), 5)
	for i := 1; i < 10; i += 2 {
		assert.Equal(t, seen[i], 1)
	}
}

func TestAddingAComponentThatAlreadyExistsIsError(t *testing.T) {
	world := newWorldForTest(t)
	energy := ecs.NewComponentType[EnergyComponent]()
	world.RegisterComponents(energy)

	ent, err := world.Create(energy)
	assert.NilError(t, err)
	assert.ErrorIs(t, energy.AddTo(ent), storage.ErrorComponentAlreadyOnEntity)
}

func TestRemovingAMissingComponentIsError(t *testing.T) {
	world := newWorldForTest(t)
	reactorEnergy := ecs.NewComponentType[EnergyComponent]()
	weaponsEnergy := ecs.NewComponentType[EnergyComponent]()
	world.RegisterComponents(reactorEnergy, weaponsEnergy)
	ent, err := world.Create(reactorEnergy)
	assert.NilError(t, err)

	assert.ErrorIs(t, weaponsEnergy.RemoveFrom(ent), storage.ErrorComponentNotOnEntity)
}

func TestVerifyAutomaticCreationOfArchetypesWorks(t *testing.T) {
	world := newWorldForTest(t)
	type Foo struct{}
	type Bar struct{}
	a, b := ecs.NewComponentType[Foo](), ecs.NewComponentType[Bar]()
	world.RegisterComponents(a, b)

	entity, err := world.Create(a, b)
	assert.NilError(t, err)

	entry, err := world.Entry(entity)
	assert.NilError(t, err)

	archIndexBefore := entry.Loc.ArchIndex

	// The entry should now be in a different archetype
	assert.NilError(t, a.RemoveFrom(entity))

	entry, err = world.Entry(entity)
	assert.NilError(t, err)

	archIndexAfter := entry.Loc.ArchIndex
	assert.Check(t, archIndexBefore != archIndexAfter)
}

func TestEntriesCanChangeTheirArchetype(t *testing.T) {
	world := newWorldForTest(t)
	type Label struct {
		Name string
	}
	alpha := ecs.NewComponentType[Label](ecs.WithDefault(Label{"alpha"}))
	beta := ecs.NewComponentType[Label](ecs.WithDefault(Label{"beta"}))
	gamma := ecs.NewComponentType[Label](ecs.WithDefault(Label{"gamma"}))
	world.RegisterComponents(alpha, beta, gamma)

	ents, err := world.CreateMany(3, alpha, beta)
	assert.NilError(t, err)

	// count and countAgain are helpers that simplify the counting of how many
	// entities have a particular component.
	var count int
	countAgain := func() func(entry *storage.Entry) {
		count = 0
		return func(entry *storage.Entry) {
			count++
		}
	}
	// 3 entities have alpha
	alpha.Each(world, countAgain())
	assert.Equal(t, 3, count)

	// 0 entities have gamma
	gamma.Each(world, countAgain())
	assert.Equal(t, 0, count)

	assert.NilError(t, alpha.RemoveFrom(ents[0]))

	// alpha has been removed from entity[0], so only 2 entities should now have alpha
	alpha.Each(world, countAgain())
	assert.Equal(t, 2, count)

	// Add gamma to an entity. Now 1 entity should have gamma.
	assert.NilError(t, gamma.AddTo(ents[1]))
	gamma.Each(world, countAgain())
	assert.Equal(t, 1, count)

	// Make sure the one entry that has gamma is ents[1]
	gamma.Each(world, func(entry *storage.Entry) {
		assert.Equal(t, entry.ID, ents[1].ID())
	})
}

func TestCannotSetComponentThatDoesNotBelongToEntity(t *testing.T) {
	world := newWorldForTest(t)

	alpha := ecs.NewComponentType[EnergyComponent]()
	beta := ecs.NewComponentType[EnergyComponent]()
	world.RegisterComponents(alpha, beta)

	e, err := world.Create(alpha)
	assert.NilError(t, err)

	entry, err := world.Entry(e)
	assert.NilError(t, err)

	err = beta.Set(entry, &EnergyComponent{100, 200})
	assert.Check(t, err != nil)
}

func TestQueriesAndFiltersWorks(t *testing.T) {
	world := newWorldForTest(t)
	a, b, c, d := ecs.NewComponentType[int](), ecs.NewComponentType[int](), ecs.NewComponentType[int](), ecs.NewComponentType[int]()
	world.RegisterComponents(a, b, c, d)

	ab, err := world.Create(a, b)
	assert.NilError(t, err)
	cd, err := world.Create(c, d)
	assert.NilError(t, err)
	_, err = world.Create(b, d)
	assert.NilError(t, err)

	// Only one entity has the components a and b
	abFilter := filter.Contains(a, b)
	ecs.NewQuery(abFilter).Each(world, func(entry *storage.Entry) {
		assert.Equal(t, entry.ID, ab.ID())
	})
	assert.Equal(t, ecs.NewQuery(abFilter).Count(world), 1)

	cdFilter := filter.Contains(c, d)
	ecs.NewQuery(cdFilter).Each(world, func(entry *storage.Entry) {
		assert.Equal(t, entry.ID, cd.ID())
	})
	assert.Equal(t, ecs.NewQuery(abFilter).Count(world), 1)

	allCount := ecs.NewQuery(filter.Or(filter.Contains(a), filter.Contains(d))).Count(world)
	assert.Equal(t, allCount, 3)
}
