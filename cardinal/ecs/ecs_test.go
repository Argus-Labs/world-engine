package ecs_test

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

type EnergyComponent struct {
	Amt int64
	Cap int64
}

func (e EnergyComponent) Name() string {
	return "EnergyComponent"
}

type OwnableComponent struct {
	Owner string
}

func UpdateEnergySystem(w *ecs.World, tq *transaction.TxQueue, _ *log.Logger) error {
	errs := []error{}

	Energy.Each(w, func(ent entity.ID) bool {
		energyPlanet, err := ecs.GetComponent[EnergyComponent](w, ent)
		//energyPlanet, err := Energy.Get(w, ent)
		if err != nil {
			errs = append(errs, err)
		}
		energyPlanet.Amt += 10 // bs whatever
		err = ecs.SetComponent[EnergyComponent](w, ent, energyPlanet)
		//err = Energy.Set(w, ent, *energyPlanet)
		if err != nil {
			errs = append(errs, err)
		}
		return true
	})
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

var (
	Energy  = ecs.NewComponentType[EnergyComponent]("EnergyComponent")
	Ownable = ecs.NewComponentType[OwnableComponent]("OwnableComponent")
)

func TestECS(t *testing.T) {
	world := ecs.NewTestWorld(t)
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

	Energy.Each(world, func(id entity.ID) bool {
		energyPlanet, err := ecs.GetComponent[EnergyComponent](world, id)
		//energyPlanet, err := Energy.Get(world, id)
		assert.NilError(t, err)
		assert.Equal(t, int64(10), energyPlanet.Amt)
		return true
	})

	q := ecs.NewQuery(filter.Or(filter.Contains(Energy), filter.Contains(Ownable)))
	amt := q.Count(world)
	assert.Equal(t, numPlanets+numEnergyOnly, amt)
	comp, exists := world.GetComponentByName("EnergyComponent")
	assert.Assert(t, exists)
	assert.Equal(t, comp.Name(), Energy.Name())
}

type Pos struct {
	X, Y float64
}
type Vel struct {
	DX, DY float64
}

func (_ Pos) Name() string {
	return "Position"
}

func (_ Vel) Name() string {
	return "Velocity"
}

func TestVelocitySimulation(t *testing.T) {
	world := ecs.NewTestWorld(t)

	// These components are a mix of concrete types and pointer types to make sure they both work
	Position := ecs.NewComponentType[Pos]("Position")
	Velocity := ecs.NewComponentType[*Vel]("Velocity")
	assert.NilError(t, world.RegisterComponents(Position, Velocity))
	assert.NilError(t, world.LoadGameState())

	shipID, err := world.Create(Position, Velocity)
	assert.NilError(t, err)
	assert.NilError(t, ecs.SetComponent[Pos](world, shipID, &Pos{1, 2}))
	assert.NilError(t, ecs.SetComponent[Vel](world, shipID, &Vel{3, 4}))
	wantPos := Pos{4, 6}

	Velocity.Each(world, func(id entity.ID) bool {
		vel, err := ecs.GetComponent[Vel](world, id)
		//vel, err := Velocity.Get(world, id)
		assert.NilError(t, err)
		pos, err := ecs.GetComponent[Pos](world, id)
		//pos, err := Position.Get(world, id)
		assert.NilError(t, err)
		newPos := Pos{pos.X + vel.DX, pos.Y + vel.DY}
		assert.NilError(t, ecs.SetComponent[Pos](world, id, &newPos))
		return true
	})

	finalPos, err := ecs.GetComponent[Pos](world, shipID)
	//finalPos, err := Position.Get(world, shipID)
	assert.NilError(t, err)
	assert.Equal(t, wantPos, *finalPos)
}

type Owner struct {
	MyName string
}

// Additional method.
func (Owner) Name() string {
	return "owner"
}

func TestCanSetDefaultValue(t *testing.T) {
	world := ecs.NewTestWorld(t)

	wantOwner := Owner{"Jeff"}

	//Below disapears and should be handled by RegisterComponents.
	owner := ecs.NewComponentType[Owner]("owner", ecs.WithDefault(wantOwner))
	assert.NilError(t, world.RegisterComponents(owner))
	assert.NilError(t, world.LoadGameState())

	alpha, err := world.Create(owner)
	assert.NilError(t, err)

	//alphaOwner, err := owner.Get(world, alpha)
	alphaOwner, err := ecs.GetComponent[Owner](world, alpha)
	assert.NilError(t, err)
	assert.Equal(t, *alphaOwner, wantOwner)

	alphaOwner.MyName = "Bob"
	//assert.NilError(t, owner.Set(world, alpha, *alphaOwner))
	assert.NilError(t, ecs.SetComponent[Owner](world, alpha, alphaOwner))

	//newOwner, err := owner.Get(world, alpha)
	newOwner, err := ecs.GetComponent[Owner](world, alpha)
	assert.NilError(t, err)
	assert.Equal(t, newOwner.MyName, "Bob")
}

type Tuple struct {
	A, B int
}

func (_ Tuple) Name() string {
	return "tuple"
}

func TestCanRemoveEntity(t *testing.T) {
	world := ecs.NewTestWorld(t)

	tuple := ecs.NewComponentType[Tuple]("tuple")
	assert.NilError(t, world.RegisterComponents(tuple))
	assert.NilError(t, world.LoadGameState())

	entities, err := world.CreateMany(2, tuple)
	assert.NilError(t, err)

	// Make sure we find exactly 2 entries
	count := 0
	tuple.Each(world, func(id entity.ID) bool {
		_, err := ecs.GetComponent[Tuple](world, id)
		//_, err := tuple.Get(world, id)
		assert.NilError(t, err)
		count++
		return true
	})

	assert.Equal(t, count, 2)
	err = world.Remove(entities[0])
	assert.NilError(t, err)

	// Now we should only find 1 entity
	count = 0
	tuple.Each(world, func(id entity.ID) bool {
		_, err := ecs.GetComponent[Tuple](world, id)
		//_, err := tuple.Get(world, id)
		assert.NilError(t, err)
		count++
		return true
	})
	assert.Equal(t, count, 1)

	// This entity was Removed, so we shouldn't be able to find it
	_, err = world.StoreManager().GetEntity(entities[0])
	assert.Check(t, err != nil)

	// Remove the other entity
	err = world.Remove(entities[1])
	assert.NilError(t, err)
	count = 0
	tuple.Each(world, func(id entity.ID) bool {
		_, err := ecs.GetComponent[Tuple](world, id)
		//_, err := tuple.Get(world, id)
		assert.NilError(t, err)
		count++
		return true
	})
	assert.Equal(t, count, 0)

	// This entity was Removed, so we shouldn't be able to find it
	_, err = world.StoreManager().GetEntity(entities[0])
	assert.Check(t, err != nil)
}

type CountComponent struct {
	Val int
}

func (_ CountComponent) Name() string {
	return "Count"
}

func TestCanRemoveEntriesDuringCallToEach(t *testing.T) {
	world := ecs.NewTestWorld(t)

	Count := ecs.NewComponentType[CountComponent]("Count")
	assert.NilError(t, world.RegisterComponents(Count))
	assert.NilError(t, world.LoadGameState())

	_, err := world.CreateMany(10, Count)
	assert.NilError(t, err)

	// Pre-populate all the entities with their own IDs. This will help
	// us keep track of which component belongs to which entity in the case
	// of a problem
	Count.Each(world, func(id entity.ID) bool {
		assert.NilError(t, ecs.SetComponent[CountComponent](world, id, &CountComponent{int(id)}))
		//assert.NilError(t, Count.Set(world, id, CountComponent{int(id)}))
		return true
	})

	// Remove the even entries
	itr := 0
	Count.Each(world, func(id entity.ID) bool {
		if itr%2 == 0 {
			assert.NilError(t, world.Remove(id))
		}
		itr++
		return true
	})
	// Verify we did this Each the correct number of times
	assert.Equal(t, 10, itr)

	seen := map[int]int{}
	Count.Each(world, func(id entity.ID) bool {
		c, err := ecs.GetComponent[CountComponent](world, id)
		//c, err := Count.Get(world, id)
		assert.NilError(t, err)
		seen[c.Val]++
		return true
	})

	// Verify we're left with exactly 5 odd values between 1 and 9
	assert.Equal(t, len(seen), 5)
	for i := 1; i < 10; i += 2 {
		assert.Equal(t, seen[i], 1)
	}
}

func TestAddingAComponentThatAlreadyExistsIsError(t *testing.T) {
	world := ecs.NewTestWorld(t)
	energy := ecs.NewComponentType[EnergyComponent]("EnergyComponent")
	assert.NilError(t, world.RegisterComponents(energy))
	assert.NilError(t, world.LoadGameState())

	ent, err := world.Create(energy)
	assert.NilError(t, err)
	assert.ErrorIs(t, ecs.AddToComponent[EnergyComponent](world, ent), storage.ErrorComponentAlreadyOnEntity)
}

type ReactorEnergy struct {
	Amt int64
	Cap int64
}

type WeaponEnergy struct {
	Amt int64
	Cap int64
}

func (ReactorEnergy) Name() string {
	return "reactorEnergy"
}

func (WeaponEnergy) Name() string {
	return "weaponsEnergy"
}

func TestRemovingAMissingComponentIsError(t *testing.T) {
	world := ecs.NewTestWorld(t)
	reactorEnergy := ecs.NewComponentType[ReactorEnergy]("reactorEnergy")
	weaponsEnergy := ecs.NewComponentType[WeaponEnergy]("weaponsEnergy")
	assert.NilError(t, world.RegisterComponents(reactorEnergy, weaponsEnergy))
	assert.NilError(t, world.LoadGameState())
	ent, err := world.Create(reactorEnergy)
	assert.NilError(t, err)

	//assert.ErrorIs(t, weaponsEnergy.RemoveFrom(world, ent), storage.ErrorComponentNotOnEntity)
	assert.ErrorIs(t, ecs.RemoveFromComponent[WeaponEnergy](world, ent), storage.ErrorComponentNotOnEntity)
}

type Foo struct{}
type Bar struct{}

func (Foo) Name() string {
	return "a"
}

func (Bar) Name() string {
	return "b"
}

func TestVerifyAutomaticCreationOfArchetypesWorks(t *testing.T) {
	world := ecs.NewTestWorld(t)

	a, b := ecs.NewComponentType[Foo]("a"), ecs.NewComponentType[Bar]("b")
	assert.NilError(t, world.RegisterComponents(a, b))
	assert.NilError(t, world.LoadGameState())

	entity, err := world.Create(a, b)
	assert.NilError(t, err)

	ent, err := world.StoreManager().GetEntity(entity)
	assert.NilError(t, err)

	archIDBefore := ent.Loc.ArchID

	// The entity should now be in a different archetype
	assert.NilError(t, ecs.RemoveFromComponent[Foo](world, entity))

	ent, err = world.StoreManager().GetEntity(entity)
	assert.NilError(t, err)

	archIDAfter := ent.Loc.ArchID
	assert.Check(t, archIDBefore != archIDAfter)
}

type Alpha struct {
	Name1 string
}
type Beta struct {
	Name1 string
}
type Gamma struct {
	Name1 string
}

func (Alpha) Name() string {
	return "alpha"
}

func (Beta) Name() string {
	return "beta"
}

func (Gamma) Name() string {
	return "gamma"
}

func TestEntriesCanChangeTheirArchetype(t *testing.T) {
	world := ecs.NewTestWorld(t)
	alpha := ecs.NewComponentType[Alpha]("alpha", ecs.WithDefault(Alpha{"alpha"}))
	beta := ecs.NewComponentType[Beta]("beta", ecs.WithDefault(Beta{"beta"}))
	gamma := ecs.NewComponentType[Gamma]("gamma", ecs.WithDefault(Gamma{"gamma"}))
	assert.NilError(t, world.RegisterComponents(alpha, beta, gamma))
	assert.NilError(t, world.LoadGameState())

	entIDs, err := world.CreateMany(3, alpha, beta)
	assert.NilError(t, err)

	// count and countAgain are helpers that simplify the counting of how many
	// entities have a particular component.
	var count int
	countAgain := func() func(ent entity.ID) bool {
		count = 0
		return func(ent entity.ID) bool {
			count++
			return true
		}
	}
	// 3 entities have alpha
	alpha.Each(world, countAgain())
	assert.Equal(t, 3, count)

	// 0 entities have gamma
	gamma.Each(world, countAgain())
	assert.Equal(t, 0, count)

	assert.NilError(t, ecs.RemoveFromComponent[Alpha](world, entIDs[0]))

	// alpha has been removed from entity[0], so only 2 entities should now have alpha
	alpha.Each(world, countAgain())
	assert.Equal(t, 2, count)

	// Add gamma to an entity. Now 1 entity should have gamma.
	assert.NilError(t, ecs.AddToComponent[Gamma](world, entIDs[1]))
	gamma.Each(world, countAgain())
	assert.Equal(t, 1, count)

	// Make sure the one ent that has gamma is entIDs[1]
	gamma.Each(world, func(id entity.ID) bool {
		assert.Equal(t, id, entIDs[1])
		return true
	})
}

type EnergyComponentAlpha struct {
	Amt int64
	Cap int64
}

func (e EnergyComponentAlpha) Name() string {
	return "alpha"
}

type EnergyComponentBeta struct {
	Amt int64
	Cap int64
}

func (e EnergyComponentBeta) Name() string {
	return "beta"
}

func TestCannotSetComponentThatDoesNotBelongToEntity(t *testing.T) {
	world := ecs.NewTestWorld(t)

	alpha := ecs.NewComponentType[EnergyComponentAlpha]("alpha")
	beta := ecs.NewComponentType[EnergyComponentBeta]("beta")
	assert.NilError(t, world.RegisterComponents(alpha, beta))
	assert.NilError(t, world.LoadGameState())

	id, err := world.Create(alpha)
	assert.NilError(t, err)

	err = ecs.SetComponent[EnergyComponentBeta](world, id, &EnergyComponentBeta{100, 200})
	//err = beta.Set(world, id, EnergyComponentBeta{100, 200})
	assert.Check(t, err != nil)
}

func TestQueriesAndFiltersWorks(t *testing.T) {
	world := ecs.NewTestWorld(t)
	a, b, c, d := ecs.NewComponentType[int]("a"), ecs.NewComponentType[int]("b"), ecs.NewComponentType[int]("c"), ecs.NewComponentType[int]("d")
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
	ecs.NewQuery(abFilter).Each(world, func(id entity.ID) bool {
		assert.Equal(t, id, ab)
		return true
	})
	assert.Equal(t, ecs.NewQuery(abFilter).Count(world), 1)

	cdFilter := filter.Contains(c, d)
	ecs.NewQuery(cdFilter).Each(world, func(id entity.ID) bool {
		assert.Equal(t, id, cd)
		return true
	})
	assert.Equal(t, ecs.NewQuery(abFilter).Count(world), 1)

	allCount := ecs.NewQuery(filter.Or(filter.Contains(a), filter.Contains(d))).Count(world)
	assert.Equal(t, allCount, 3)
}

type HealthComponent struct {
	HP int
}

func (_ HealthComponent) Name() string {
	return "hpComp"
}

func TestUpdateWithPointerType(t *testing.T) {

	world := ecs.NewTestWorld(t)
	hpComp := ecs.NewComponentType[*HealthComponent]("hpComp")
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

	hp, err := ecs.GetComponent[HealthComponent](world, id)
	//hp, err := hpComp.Get(world, id)
	assert.NilError(t, err)
	assert.Equal(t, 100, hp.HP)
}

type ValueComponent1 struct {
	Val int
}

func (_ ValueComponent1) Name() string {
	return "valComp"
}

func TestCanRemoveFirstEntity(t *testing.T) {

	world := ecs.NewTestWorld(t)
	valComp := ecs.NewComponentType[ValueComponent1]("valComp")
	assert.NilError(t, world.RegisterComponents(valComp))

	ids, err := world.CreateMany(3, valComp)
	assert.NilError(t, err)
	assert.NilError(t, ecs.SetComponent[ValueComponent1](world, ids[0], &ValueComponent1{99}))
	assert.NilError(t, ecs.SetComponent[ValueComponent1](world, ids[1], &ValueComponent1{100}))
	assert.NilError(t, ecs.SetComponent[ValueComponent1](world, ids[2], &ValueComponent1{101}))

	assert.NilError(t, world.Remove(ids[0]))

	val, err := ecs.GetComponent[ValueComponent1](world, ids[1])
	//val, err := valComp.Get(world, ids[1])
	assert.NilError(t, err)
	assert.Equal(t, 100, val.Val)

	val, err = ecs.GetComponent[ValueComponent1](world, ids[2])
	//val, err = valComp.Get(world, ids[2])
	assert.NilError(t, err)
	assert.Equal(t, 101, val.Val)
}

type ValueComponent2 struct {
	Val int
}
type OtherComponent struct {
	Val int
}

func (_ ValueComponent2) Name() string {
	return "valComp"
}

func (_ OtherComponent) Name() string {
	return "otherComp"
}

func TestCanChangeArchetypeOfFirstEntity(t *testing.T) {

	world := ecs.NewTestWorld(t)
	valComp := ecs.NewComponentType[ValueComponent2]("valComp")
	otherComp := ecs.NewComponentType[OtherComponent]("otherComp")
	assert.NilError(t, world.RegisterComponents(valComp, otherComp))

	ids, err := world.CreateMany(3, valComp)
	assert.NilError(t, err)
	assert.NilError(t, ecs.SetComponent[ValueComponent2](world, ids[0], &ValueComponent2{99}))
	assert.NilError(t, ecs.SetComponent[ValueComponent2](world, ids[1], &ValueComponent2{100}))
	assert.NilError(t, ecs.SetComponent[ValueComponent2](world, ids[2], &ValueComponent2{101}))

	assert.NilError(t, ecs.AddToComponent[OtherComponent](world, ids[0]))

	val, err := ecs.GetComponent[ValueComponent2](world, ids[1])
	//val, err := valComp.Get(world, ids[1])
	assert.NilError(t, err)
	assert.Equal(t, 100, val.Val)

	val, err = ecs.GetComponent[ValueComponent2](world, ids[2])
	//val, err = valComp.Get(world, ids[2])
	assert.NilError(t, err)
	assert.Equal(t, 101, val.Val)
}
