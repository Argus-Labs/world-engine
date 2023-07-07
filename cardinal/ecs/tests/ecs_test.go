package tests

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

type EnergyComponent struct {
	Amt int64
	Cap int64
}

type OwnableComponent struct {
	Owner string
}

func UpdateEnergySystem(w *ecs.World, tq *ecs.TransactionQueue) error {
	errs := []error{}

	Energy.Each(w, func(ent storage.EntityID) {
		energyPlanet, err := Energy.Get(w, ent)
		if err != nil {
			errs = append(errs, err)
		}
		energyPlanet.Amt += 10 // bs whatever
		err = Energy.Set(w, ent, energyPlanet)
		if err != nil {
			errs = append(errs, err)
		}
	})
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

var (
	Energy  = ecs.NewComponentType[EnergyComponent]()
	Ownable = ecs.NewComponentType[OwnableComponent]()
)

func TestECS(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.RegisterComponents(Energy, Ownable))

	// create a bunch of planets!
	numPlanets := 5
	_, err := world.CreateMany(numPlanets, Energy, Ownable)
	assert.NilError(t, err)

	numEnergyOnly := 10
	_, err = world.CreateMany(numEnergyOnly, Energy)
	assert.NilError(t, err)

	world.AddSystem(UpdateEnergySystem)
	assert.NilError(t, world.LoadGameState())

	assert.NilError(t, world.Tick(context.Background()))

	Energy.Each(world, func(id storage.EntityID) {
		energyPlanet, err := Energy.Get(world, id)
		assert.NilError(t, err)
		assert.Equal(t, int64(10), energyPlanet.Amt)
	})

	q := ecs.NewQuery(filter.Or(filter.Contains(Energy), filter.Contains(Ownable)))
	amt := q.Count(world)
	assert.Equal(t, numPlanets+numEnergyOnly, amt)
}

func TestVelocitySimulation(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	type Pos struct {
		X, Y float64
	}
	type Vel struct {
		DX, DY float64
	}
	// These components are a mix of concrete types and pointer types to make sure they both work
	Position := ecs.NewComponentType[Pos]()
	Velocity := ecs.NewComponentType[*Vel]()
	assert.NilError(t, world.RegisterComponents(Position, Velocity))
	assert.NilError(t, world.LoadGameState())

	shipID, err := world.Create(Position, Velocity)
	assert.NilError(t, err)
	assert.NilError(t, Position.Set(world, shipID, Pos{1, 2}))
	assert.NilError(t, Velocity.Set(world, shipID, &Vel{3, 4}))
	wantPos := Pos{4, 6}

	Velocity.Each(world, func(id storage.EntityID) {
		vel, err := Velocity.Get(world, id)
		assert.NilError(t, err)
		pos, err := Position.Get(world, id)
		assert.NilError(t, err)
		newPos := Pos{pos.X + vel.DX, pos.Y + vel.DY}
		assert.NilError(t, Position.Set(world, id, newPos))
	})

	finalPos, err := Position.Get(world, shipID)
	assert.NilError(t, err)
	assert.Equal(t, wantPos, finalPos)
}

func TestCanSetDefaultValue(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	type Owner struct {
		Name string
	}
	wantOwner := Owner{"Jeff"}
	owner := ecs.NewComponentType[Owner](ecs.WithDefault(wantOwner))
	assert.NilError(t, world.RegisterComponents(owner))
	assert.NilError(t, world.LoadGameState())

	alpha, err := world.Create(owner)
	assert.NilError(t, err)

	alphaOwner, err := owner.Get(world, alpha)
	assert.NilError(t, err)
	assert.Equal(t, alphaOwner, wantOwner)

	alphaOwner.Name = "Bob"
	assert.NilError(t, owner.Set(world, alpha, alphaOwner))

	newOwner, err := owner.Get(world, alpha)
	assert.Equal(t, newOwner.Name, "Bob")
}

func TestCanRemoveEntity(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	type Tuple struct {
		A, B int
	}

	tuple := ecs.NewComponentType[Tuple]()
	assert.NilError(t, world.RegisterComponents(tuple))
	assert.NilError(t, world.LoadGameState())

	entities, err := world.CreateMany(2, tuple)
	assert.NilError(t, err)

	// Make sure we find exactly 2 entries
	count := 0
	tuple.Each(world, func(id storage.EntityID) {
		_, err := tuple.Get(world, id)
		assert.NilError(t, err)
		count++
	})

	assert.Equal(t, count, 2)
	err = world.Remove(entities[0])
	assert.NilError(t, err)

	// Now we should only find 1 entity
	count = 0
	tuple.Each(world, func(id storage.EntityID) {
		_, err := tuple.Get(world, id)
		assert.NilError(t, err)
		count++
	})
	assert.Equal(t, count, 1)

	// This entity was Removed, so we shouldn't be able to find it
	_, err = world.Entity(entities[0])
	assert.Check(t, err != nil)

	// Remove the other entity
	err = world.Remove(entities[1])
	assert.NilError(t, err)
	count = 0
	tuple.Each(world, func(id storage.EntityID) {
		_, err := tuple.Get(world, id)
		assert.NilError(t, err)
		count++
	})
	assert.Equal(t, count, 0)

	// This entity was Removed, so we shouldn't be able to find it
	_, err = world.Entity(entities[0])
	assert.Check(t, err != nil)
}

func TestCanRemoveEntriesDuringCallToEach(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	type CountComponent struct {
		Val int
	}
	Count := ecs.NewComponentType[CountComponent]()
	assert.NilError(t, world.RegisterComponents(Count))
	assert.NilError(t, world.LoadGameState())

	_, err := world.CreateMany(10, Count)
	assert.NilError(t, err)

	// Pre-populate all the entities with their own IDs. This will help
	// us keep track of which component belongs to which entity in the case
	// of a problem
	Count.Each(world, func(id storage.EntityID) {
		assert.NilError(t, Count.Set(world, id, CountComponent{int(id)}))
	})

	// Remove the even entries
	itr := 0
	Count.Each(world, func(id storage.EntityID) {
		if itr%2 == 0 {
			assert.NilError(t, world.Remove(id))
		}
		itr++
	})
	// Verify we did this Each the correct number of times
	assert.Equal(t, 10, itr)

	seen := map[int]int{}
	Count.Each(world, func(id storage.EntityID) {
		c, err := Count.Get(world, id)
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
	world := inmem.NewECSWorldForTest(t)
	energy := ecs.NewComponentType[EnergyComponent]()
	assert.NilError(t, world.RegisterComponents(energy))
	assert.NilError(t, world.LoadGameState())

	ent, err := world.Create(energy)
	assert.NilError(t, err)
	assert.ErrorIs(t, energy.AddTo(world, ent), storage.ErrorComponentAlreadyOnEntity)
}

func TestRemovingAMissingComponentIsError(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	reactorEnergy := ecs.NewComponentType[EnergyComponent]()
	weaponsEnergy := ecs.NewComponentType[EnergyComponent]()
	assert.NilError(t, world.RegisterComponents(reactorEnergy, weaponsEnergy))
	assert.NilError(t, world.LoadGameState())
	ent, err := world.Create(reactorEnergy)
	assert.NilError(t, err)

	assert.ErrorIs(t, weaponsEnergy.RemoveFrom(world, ent), storage.ErrorComponentNotOnEntity)
}

func TestVerifyAutomaticCreationOfArchetypesWorks(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	type Foo struct{}
	type Bar struct{}
	a, b := ecs.NewComponentType[Foo](), ecs.NewComponentType[Bar]()
	assert.NilError(t, world.RegisterComponents(a, b))
	assert.NilError(t, world.LoadGameState())

	entity, err := world.Create(a, b)
	assert.NilError(t, err)

	ent, err := world.Entity(entity)
	assert.NilError(t, err)

	archIDBefore := ent.Loc.ArchID

	// The entity should now be in a different archetype
	assert.NilError(t, a.RemoveFrom(world, entity))

	ent, err = world.Entity(entity)
	assert.NilError(t, err)

	archIDAfter := ent.Loc.ArchID
	assert.Check(t, archIDBefore != archIDAfter)
}

func TestEntriesCanChangeTheirArchetype(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	type Label struct {
		Name string
	}
	alpha := ecs.NewComponentType[Label](ecs.WithDefault(Label{"alpha"}))
	beta := ecs.NewComponentType[Label](ecs.WithDefault(Label{"beta"}))
	gamma := ecs.NewComponentType[Label](ecs.WithDefault(Label{"gamma"}))
	assert.NilError(t, world.RegisterComponents(alpha, beta, gamma))
	assert.NilError(t, world.LoadGameState())

	entIDs, err := world.CreateMany(3, alpha, beta)
	assert.NilError(t, err)

	// count and countAgain are helpers that simplify the counting of how many
	// entities have a particular component.
	var count int
	countAgain := func() func(ent storage.EntityID) {
		count = 0
		return func(ent storage.EntityID) {
			count++
		}
	}
	// 3 entities have alpha
	alpha.Each(world, countAgain())
	assert.Equal(t, 3, count)

	// 0 entities have gamma
	gamma.Each(world, countAgain())
	assert.Equal(t, 0, count)

	assert.NilError(t, alpha.RemoveFrom(world, entIDs[0]))

	// alpha has been removed from entity[0], so only 2 entities should now have alpha
	alpha.Each(world, countAgain())
	assert.Equal(t, 2, count)

	// Add gamma to an entity. Now 1 entity should have gamma.
	assert.NilError(t, gamma.AddTo(world, entIDs[1]))
	gamma.Each(world, countAgain())
	assert.Equal(t, 1, count)

	// Make sure the one ent that has gamma is entIDs[1]
	gamma.Each(world, func(id storage.EntityID) {
		assert.Equal(t, id, entIDs[1])
	})
}

func TestCannotSetComponentThatDoesNotBelongToEntity(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)

	alpha := ecs.NewComponentType[EnergyComponent]()
	beta := ecs.NewComponentType[EnergyComponent]()
	assert.NilError(t, world.RegisterComponents(alpha, beta))
	assert.NilError(t, world.LoadGameState())

	id, err := world.Create(alpha)
	assert.NilError(t, err)

	err = beta.Set(world, id, EnergyComponent{100, 200})
	assert.Check(t, err != nil)
}

func TestQueriesAndFiltersWorks(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	a, b, c, d := ecs.NewComponentType[int](), ecs.NewComponentType[int](), ecs.NewComponentType[int](), ecs.NewComponentType[int]()
	assert.NilError(t, world.RegisterComponents(a, b, c, d))
	assert.NilError(t, world.LoadGameState())

	ab, err := world.Create(a, b)
	assert.NilError(t, err)
	cd, err := world.Create(c, d)
	assert.NilError(t, err)
	_, err = world.Create(b, d)
	assert.NilError(t, err)

	// Only one entity has the components a and b
	abFilter := filter.Contains(a, b)
	ecs.NewQuery(abFilter).Each(world, func(id storage.EntityID) {
		assert.Equal(t, id, ab)
	})
	assert.Equal(t, ecs.NewQuery(abFilter).Count(world), 1)

	cdFilter := filter.Contains(c, d)
	ecs.NewQuery(cdFilter).Each(world, func(id storage.EntityID) {
		assert.Equal(t, id, cd)
	})
	assert.Equal(t, ecs.NewQuery(abFilter).Count(world), 1)

	allCount := ecs.NewQuery(filter.Or(filter.Contains(a), filter.Contains(d))).Count(world)
	assert.Equal(t, allCount, 3)
}

func TestUpdateWithPointerType(t *testing.T) {
	type HealthComponent struct {
		HP int
	}
	world := inmem.NewECSWorldForTest(t)
	hpComp := ecs.NewComponentType[*HealthComponent]()
	assert.NilError(t, world.RegisterComponents(hpComp))
	assert.NilError(t, world.LoadGameState())

	id, err := world.Create(hpComp)
	assert.NilError(t, err)

	hpComp.Update(world, id, func(h *HealthComponent) *HealthComponent {
		if h == nil {
			h = &HealthComponent{}
		}
		h.HP += 100
		return h
	})

	hp, err := hpComp.Get(world, id)
	assert.NilError(t, err)
	assert.Equal(t, 100, hp.HP)
}

func TestCanRemoveFirstEntity(t *testing.T) {
	type ValueComponent struct {
		Val int
	}
	world := inmem.NewECSWorldForTest(t)
	valComp := ecs.NewComponentType[ValueComponent]()
	assert.NilError(t, world.RegisterComponents(valComp))

	ids, err := world.CreateMany(3, valComp)
	assert.NilError(t, err)
	assert.NilError(t, valComp.Set(world, ids[0], ValueComponent{99}))
	assert.NilError(t, valComp.Set(world, ids[1], ValueComponent{100}))
	assert.NilError(t, valComp.Set(world, ids[2], ValueComponent{101}))

	assert.NilError(t, world.Remove(ids[0]))

	val, err := valComp.Get(world, ids[1])
	assert.NilError(t, err)
	assert.Equal(t, 100, val.Val)

	val, err = valComp.Get(world, ids[2])
	assert.NilError(t, err)
	assert.Equal(t, 101, val.Val)
}

func TestCanChangeArchetypeOfFirstEntity(t *testing.T) {
	type ValueComponent struct {
		Val int
	}
	type OtherComponent struct {
		Val int
	}
	world := inmem.NewECSWorldForTest(t)
	valComp := ecs.NewComponentType[ValueComponent]()
	otherComp := ecs.NewComponentType[OtherComponent]()
	assert.NilError(t, world.RegisterComponents(valComp, otherComp))

	ids, err := world.CreateMany(3, valComp)
	assert.NilError(t, err)
	assert.NilError(t, valComp.Set(world, ids[0], ValueComponent{99}))
	assert.NilError(t, valComp.Set(world, ids[1], ValueComponent{100}))
	assert.NilError(t, valComp.Set(world, ids[2], ValueComponent{101}))

	assert.NilError(t, otherComp.AddTo(world, ids[0]))

	val, err := valComp.Get(world, ids[1])
	assert.NilError(t, err)
	assert.Equal(t, 100, val.Val)

	val, err = valComp.Get(world, ids[2])
	assert.NilError(t, err)
	assert.Equal(t, 101, val.Val)
}
