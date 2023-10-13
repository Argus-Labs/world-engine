package ecs_test

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/component"

	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

type EnergyComponent struct {
	Amt int64
	Cap int64
}

func (EnergyComponent) Name() string {
	return "EnergyComponent"
}

type OwnableComponent struct {
	Owner string
}

func (OwnableComponent) Name() string {
	return "OwnableComponent"
}

func UpdateEnergySystem(w *ecs.World, tq *transaction.TxQueue, _ *log.Logger) error {
	errs := []error{}
	q, err := w.NewQuery(ecs.Contains(EnergyComponent{}))
	errs = append(errs, err)
	q.Each(w, func(ent entity.ID) bool {
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
	Energy  = component.NewComponentMetaData[EnergyComponent]()
	Ownable = component.NewComponentMetaData[OwnableComponent]()
)

func TestECS(t *testing.T) {
	world := ecs.NewTestWorld(t)
	assert.NilError(t, ecs.RegisterComponent[EnergyComponent](world))
	assert.NilError(t, ecs.RegisterComponent[OwnableComponent](world))

	// create a bunch of planets!
	numPlanets := 5
	_, err := ecs.CreateMany(world, numPlanets, EnergyComponent{}, OwnableComponent{})
	assert.NilError(t, err)

	numEnergyOnly := 10
	_, err = ecs.CreateMany(world, numEnergyOnly, EnergyComponent{})
	assert.NilError(t, err)

	world.AddSystem(UpdateEnergySystem)
	assert.NilError(t, world.LoadGameState())

	assert.NilError(t, world.Tick(context.Background()))
	query, err := world.NewQuery(ecs.Contains(EnergyComponent{}))
	assert.NilError(t, err)
	query.Each(world, func(id entity.ID) bool {
		energyPlanet, err := ecs.GetComponent[EnergyComponent](world, id)
		//energyPlanet, err := Energy.Get(world, id)
		assert.NilError(t, err)
		assert.Equal(t, int64(10), energyPlanet.Amt)
		return true
	})

	q, err := world.NewQuery(ecs.Or(ecs.Contains(EnergyComponent{}), ecs.Contains(OwnableComponent{})))
	assert.NilError(t, err)
	amt := q.Count(world)
	assert.Equal(t, numPlanets+numEnergyOnly, amt)
	comp, err := world.GetComponentByName("EnergyComponent")
	assert.NilError(t, err)
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
	assert.NilError(t, ecs.RegisterComponent[Pos](world))
	assert.NilError(t, ecs.RegisterComponent[Vel](world))
	assert.NilError(t, world.LoadGameState())

	shipID, err := ecs.Create(world, Pos{}, Vel{})
	assert.NilError(t, err)
	assert.NilError(t, ecs.SetComponent[Pos](world, shipID, &Pos{1, 2}))
	assert.NilError(t, ecs.SetComponent[Vel](world, shipID, &Vel{3, 4}))
	wantPos := Pos{4, 6}

	velocityQuery, err := world.NewQuery(ecs.Contains(&Vel{}))
	assert.NilError(t, err)
	velocityQuery.Each(world, func(id entity.ID) bool {
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
	assert.NilError(t, ecs.RegisterComponent[Owner](world))
	assert.NilError(t, world.LoadGameState())

	alphaEntity, err := ecs.Create(world, wantOwner)
	assert.NilError(t, err)

	//alphaOwner, err := owner.Get(world, alpha)
	alphaOwner, err := ecs.GetComponent[Owner](world, alphaEntity)
	assert.NilError(t, err)
	assert.Equal(t, *alphaOwner, wantOwner)

	alphaOwner.MyName = "Bob"
	//assert.NilError(t, owner.Set(world, alpha, *alphaOwner))
	assert.NilError(t, ecs.SetComponent[Owner](world, alphaEntity, alphaOwner))

	//newOwner, err := owner.Get(world, alpha)
	newOwner, err := ecs.GetComponent[Owner](world, alphaEntity)
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

	assert.NilError(t, ecs.RegisterComponent[Tuple](world))
	assert.NilError(t, world.LoadGameState())

	entities, err := ecs.CreateMany(world, 2, Tuple{})
	assert.NilError(t, err)

	// Make sure we find exactly 2 entries
	count := 0
	q, err := world.NewQuery(ecs.Contains(Tuple{}))
	assert.NilError(t, err)
	q.Each(world, func(id entity.ID) bool {
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
	q.Each(world, func(id entity.ID) bool {
		_, err := ecs.GetComponent[Tuple](world, id)
		//_, err := tuple.Get(world, id)
		assert.NilError(t, err)
		count++
		return true
	})
	assert.Equal(t, count, 1)

	// This entity was Removed, so we shouldn't be able to find it
	_, err = world.StoreManager().GetComponentTypesForEntity(entities[0])
	assert.Check(t, err != nil)

	// Remove the other entity
	err = world.Remove(entities[1])
	assert.NilError(t, err)
	count = 0
	q.Each(world, func(id entity.ID) bool {
		_, err := ecs.GetComponent[Tuple](world, id)
		//_, err := tuple.Get(world, id)
		assert.NilError(t, err)
		count++
		return true
	})
	assert.Equal(t, count, 0)

	// This entity was Removed, so we shouldn't be able to find it
	_, err = world.StoreManager().GetComponentTypesForEntity(entities[0])
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

	assert.NilError(t, ecs.RegisterComponent[CountComponent](world))
	assert.NilError(t, world.LoadGameState())

	_, err := ecs.CreateMany(world, 10, CountComponent{})
	assert.NilError(t, err)

	// Pre-populate all the entities with their own IDs. This will help
	// us keep track of which component belongs to which entity in the case
	// of a problem
	q, err := world.NewQuery(ecs.Contains(CountComponent{}))
	q.Each(world, func(id entity.ID) bool {
		assert.NilError(t, ecs.SetComponent[CountComponent](world, id, &CountComponent{int(id)}))
		//assert.NilError(t, Count.Set(world, id, CountComponent{int(id)}))
		return true
	})

	// Remove the even entries
	itr := 0
	q.Each(world, func(id entity.ID) bool {
		if itr%2 == 0 {
			assert.NilError(t, world.Remove(id))
		}
		itr++
		return true
	})
	// Verify we did this Each the correct number of times
	assert.Equal(t, 10, itr)

	seen := map[int]int{}
	q.Each(world, func(id entity.ID) bool {
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
	assert.NilError(t, ecs.RegisterComponent[EnergyComponent](world))
	assert.NilError(t, world.LoadGameState())

	ent, err := ecs.Create(world, EnergyComponent{})
	assert.NilError(t, err)
	assert.ErrorIs(t, ecs.AddComponentTo[EnergyComponent](world, ent), storage.ErrorComponentAlreadyOnEntity)
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
	assert.NilError(t, ecs.RegisterComponent[ReactorEnergy](world))
	assert.NilError(t, ecs.RegisterComponent[WeaponEnergy](world))
	assert.NilError(t, world.LoadGameState())
	ent, err := ecs.Create(world, ReactorEnergy{})
	assert.NilError(t, err)

	//assert.ErrorIs(t, weaponsEnergy.RemoveFrom(world, ent), storage.ErrorComponentNotOnEntity)
	assert.ErrorIs(t, ecs.RemoveComponentFrom[WeaponEnergy](world, ent), storage.ErrorComponentNotOnEntity)
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

	assert.NilError(t, ecs.RegisterComponent[Foo](world))
	assert.NilError(t, ecs.RegisterComponent[Bar](world))

	assert.NilError(t, world.LoadGameState())

	getArchIDForEntityID := func(id entity.ID) archetype.ID {
		components, err := world.StoreManager().GetComponentTypesForEntity(id)
		assert.NilError(t, err)
		archID, err := world.StoreManager().GetArchIDForComponents(components)
		assert.NilError(t, err)
		return archID
	}
	entity, err := ecs.Create(world, Foo{}, Bar{})
	assert.NilError(t, err)

	archIDBefore := getArchIDForEntityID(entity)

	// The entity should now be in a different archetype
	assert.NilError(t, ecs.RemoveComponentFrom[Foo](world, entity))

	archIDAfter := getArchIDForEntityID(entity)
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
	assert.NilError(t, ecs.RegisterComponent[Alpha](world))
	assert.NilError(t, ecs.RegisterComponent[Beta](world))
	assert.NilError(t, ecs.RegisterComponent[Gamma](world))

	assert.NilError(t, world.LoadGameState())

	entIDs, err := ecs.CreateMany(world, 3, Alpha{}, Beta{})
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
	alphaQuery, err := world.NewQuery(ecs.Contains(Alpha{}))
	assert.NilError(t, err)
	alphaQuery.Each(world, countAgain())
	assert.Equal(t, 3, count)

	// 0 entities have gamma
	gammaQuery, err := world.NewQuery(ecs.Contains(Gamma{}))
	assert.NilError(t, err)
	gammaQuery.Each(world, countAgain())
	assert.Equal(t, 0, count)

	assert.NilError(t, ecs.RemoveComponentFrom[Alpha](world, entIDs[0]))

	// alpha has been removed from entity[0], so only 2 entities should now have alpha
	alphaQuery.Each(world, countAgain())
	assert.Equal(t, 2, count)

	// Add gamma to an entity. Now 1 entity should have gamma.
	assert.NilError(t, ecs.AddComponentTo[Gamma](world, entIDs[1]))
	gammaQuery.Each(world, countAgain())
	assert.Equal(t, 1, count)

	// Make sure the one ent that has gamma is entIDs[1]
	gammaQuery.Each(world, func(id entity.ID) bool {
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

	assert.NilError(t, ecs.RegisterComponent[EnergyComponentAlpha](world))
	assert.NilError(t, ecs.RegisterComponent[EnergyComponentBeta](world))
	assert.NilError(t, world.LoadGameState())

	id, err := ecs.Create(world, EnergyComponentAlpha{})
	assert.NilError(t, err)

	err = ecs.SetComponent[EnergyComponentBeta](world, id, &EnergyComponentBeta{100, 200})
	//err = beta.Set(world, id, EnergyComponentBeta{100, 200})
	assert.Check(t, err != nil)
}

type A struct{}
type B struct{}
type C struct{}
type D struct{}

func (A) Name() string { return "a" }
func (B) Name() string { return "b" }
func (C) Name() string { return "c" }
func (D) Name() string { return "d" }

func TestQueriesAndFiltersWorks(t *testing.T) {
	world := ecs.NewTestWorld(t)
	assert.NilError(t, ecs.RegisterComponent[A](world))
	assert.NilError(t, ecs.RegisterComponent[B](world))
	assert.NilError(t, ecs.RegisterComponent[C](world))
	assert.NilError(t, ecs.RegisterComponent[D](world))

	assert.NilError(t, world.LoadGameState())

	ab, err := ecs.Create(world, A{}, B{})
	assert.NilError(t, err)
	cd, err := ecs.Create(world, C{}, D{})
	assert.NilError(t, err)
	_, err = ecs.Create(world, B{}, D{})
	assert.NilError(t, err)

	// Only one entity has the components a and b
	abFilter := ecs.Contains(A{}, B{})
	q, err := world.NewQuery(abFilter)
	assert.NilError(t, err)
	q.Each(world, func(id entity.ID) bool {
		assert.Equal(t, id, ab)
		return true
	})
	q, err = world.NewQuery(abFilter)
	assert.NilError(t, err)
	assert.Equal(t, q.Count(world), 1)

	cdFilter := ecs.Contains(C{}, D{})
	q, err = world.NewQuery(cdFilter)
	assert.NilError(t, err)
	q.Each(world, func(id entity.ID) bool {
		assert.Equal(t, id, cd)
		return true
	})
	q, err = world.NewQuery(abFilter)
	assert.NilError(t, err)
	assert.Equal(t, q.Count(world), 1)

	q, err = world.NewQuery(ecs.Or(ecs.Contains(A{}), ecs.Contains(D{})))
	assert.NilError(t, err)
	allCount := q.Count(world)
	assert.Equal(t, allCount, 3)
}

type HealthComponent struct {
	HP int
}

func (HealthComponent) Name() string {
	return "hpComp"
}

func TestUpdateWithPointerType(t *testing.T) {

	world := ecs.NewTestWorld(t)
	assert.NilError(t, ecs.RegisterComponent[HealthComponent](world))
	assert.NilError(t, world.LoadGameState())

	id, err := ecs.Create(world, HealthComponent{})
	assert.NilError(t, err)

	err = ecs.UpdateComponent[HealthComponent](world, id, func(h *HealthComponent) *HealthComponent {
		if h == nil {
			h = &HealthComponent{}
		}
		h.HP += 100
		return h
	})
	assert.NilError(t, err)

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
	assert.NilError(t, ecs.RegisterComponent[ValueComponent1](world))

	ids, err := ecs.CreateMany(world, 3, ValueComponent1{})
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
	assert.NilError(t, ecs.RegisterComponent[ValueComponent2](world))
	assert.NilError(t, ecs.RegisterComponent[OtherComponent](world))

	ids, err := ecs.CreateMany(world, 3, ValueComponent2{})
	assert.NilError(t, err)
	assert.NilError(t, ecs.SetComponent[ValueComponent2](world, ids[0], &ValueComponent2{99}))
	assert.NilError(t, ecs.SetComponent[ValueComponent2](world, ids[1], &ValueComponent2{100}))
	assert.NilError(t, ecs.SetComponent[ValueComponent2](world, ids[2], &ValueComponent2{101}))

	assert.NilError(t, ecs.AddComponentTo[OtherComponent](world, ids[0]))

	val, err := ecs.GetComponent[ValueComponent2](world, ids[1])
	//val, err := valComp.Get(world, ids[1])
	assert.NilError(t, err)
	assert.Equal(t, 100, val.Val)

	val, err = ecs.GetComponent[ValueComponent2](world, ids[2])
	//val, err = valComp.Get(world, ids[2])
	assert.NilError(t, err)
	assert.Equal(t, 101, val.Val)
}

func TestEntityCreationAndSetting(t *testing.T) {
	world := ecs.NewTestWorld(t)
	assert.NilError(t, ecs.RegisterComponent[ValueComponent2](world))
	assert.NilError(t, ecs.RegisterComponent[OtherComponent](world))

	ids, err := ecs.CreateMany(world, 300, ValueComponent2{999}, OtherComponent{999})
	assert.NilError(t, err)
	for _, id := range ids {
		x, err := ecs.GetComponent[ValueComponent2](world, id)
		assert.NilError(t, err)
		y, err := ecs.GetComponent[OtherComponent](world, id)
		assert.NilError(t, err)
		assert.Equal(t, x.Val, 999)
		assert.Equal(t, y.Val, 999)
	}
}
