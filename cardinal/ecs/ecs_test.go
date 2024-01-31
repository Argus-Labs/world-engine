package ecs_test

import (
	"context"
	"errors"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs/search"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/iterators"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types/archetype"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

type EnergyComponent struct {
	Amt int64
	Cap int64
}

func (EnergyComponent) Name() string {
	return "EnergyComponent"
}

type AlteredEnergyComponent struct {
	Amt        int64
	Cap        int64
	ExtraThing int64
}

func (AlteredEnergyComponent) Name() string {
	return "EnergyComponent"
}

type OwnableComponent struct {
	Owner string
}

func (OwnableComponent) Name() string {
	return "OwnableComponent"
}

func UpdateEnergySystem(eCtx engine.Context) error {
	var errs []error
	q := search.NewSearch(filter.Contains(EnergyComponent{}), eCtx.Namespace(), eCtx.StoreReader())
	err := q.Each(
		func(ent entity.ID) bool {
			energyPlanet, err := ecs.GetComponent[EnergyComponent](eCtx, ent)
			if err != nil {
				errs = append(errs, err)
			}
			energyPlanet.Amt += 10 // bs whatever
			err = ecs.SetComponent[EnergyComponent](eCtx, ent, energyPlanet)
			if err != nil {
				errs = append(errs, err)
			}
			return true
		},
	)
	if err != nil {
		return err
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

var (
	Energy, errForEnergy   = component.NewComponentMetadata[EnergyComponent]()
	Ownable, errForOwnable = component.NewComponentMetadata[OwnableComponent]()
)

func TestGlobals(t *testing.T) {
	assert.NilError(t, errForEnergy)
	assert.NilError(t, errForOwnable)
}

func TestSchemaChecking(t *testing.T) {
	s := miniredis.RunT(t)

	cardinalWorld := testutils.NewTestFixture(t, s).World
	world := cardinalWorld
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponent](world))
	assert.NilError(t, cardinal.RegisterComponent[OwnableComponent](world))

	assert.NilError(t, world.LoadGameState())

	cardinalWorld2 := testutils.NewTestFixture(t, s).World
	world2 := cardinalWorld2
	assert.NilError(t, cardinal.RegisterComponent[OwnableComponent](world2))
	assert.Assert(t, cardinal.RegisterComponent[AlteredEnergyComponent](world2) != nil)
	err := cardinalWorld2.Shutdown()
	assert.NilError(t, err)
	err = cardinalWorld.Shutdown()
	assert.NilError(t, err)
}

func TestECS(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponent](world))
	assert.NilError(t, cardinal.RegisterComponent[OwnableComponent](world))

	// create a bunch of planets!
	numPlanets := 5
	eCtx := cardinal.NewWorldContext(world)

	err := cardinal.RegisterSystems(world, UpdateEnergySystem)
	assert.NilError(t, err)
	assert.NilError(t, world.LoadGameState())
	numEnergyOnly := 10
	_, err = ecs.CreateMany(eCtx, numEnergyOnly, EnergyComponent{})
	assert.NilError(t, err)
	_, err = ecs.CreateMany(eCtx, numPlanets, EnergyComponent{}, OwnableComponent{})
	assert.NilError(t, err)

	assert.NilError(t, world.Tick(context.Background()))
	query := cardinal.NewSearch(eCtx, filter.Contains(EnergyComponent{}))
	err = query.Each(
		func(id entity.ID) bool {
			energyPlanet, err := ecs.GetComponent[EnergyComponent](eCtx, id)
			assert.NilError(t, err)
			assert.Equal(t, int64(10), energyPlanet.Amt)
			return true
		},
	)
	assert.NilError(t, err)

	q := cardinal.NewSearch(eCtx, filter.Or(filter.Contains(EnergyComponent{}), filter.Contains(OwnableComponent{})))
	amt, err := q.Count()
	assert.NilError(t, err)
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

func (Pos) Name() string {
	return "Position"
}

func (Vel) Name() string {
	return "Velocity"
}

func TestVelocitySimulation(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World

	// These components are a mix of concrete types and pointer types to make sure they both work
	assert.NilError(t, cardinal.RegisterComponent[Pos](world))
	assert.NilError(t, cardinal.RegisterComponent[Vel](world))
	assert.NilError(t, world.LoadGameState())

	eCtx := cardinal.NewWorldContext(world)
	shipID, err := ecs.Create(eCtx, Pos{}, Vel{})
	assert.NilError(t, err)
	assert.NilError(t, ecs.SetComponent[Pos](eCtx, shipID, &Pos{1, 2}))
	assert.NilError(t, ecs.SetComponent[Vel](eCtx, shipID, &Vel{3, 4}))
	wantPos := Pos{4, 6}

	velocityQuery := cardinal.NewSearch(eCtx, filter.Contains(&Vel{}))
	err = velocityQuery.Each(
		func(id entity.ID) bool {
			vel, err := ecs.GetComponent[Vel](eCtx, id)
			assert.NilError(t, err)
			pos, err := ecs.GetComponent[Pos](eCtx, id)
			assert.NilError(t, err)
			newPos := Pos{pos.X + vel.DX, pos.Y + vel.DY}
			assert.NilError(t, ecs.SetComponent[Pos](eCtx, id, &newPos))
			return true
		},
	)
	assert.NilError(t, err)

	finalPos, err := ecs.GetComponent[Pos](eCtx, shipID)
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
	world := testutils.NewTestFixture(t, nil).World

	wantOwner := Owner{"Jeff"}

	assert.NilError(t, cardinal.RegisterComponent[Owner](world))
	assert.NilError(t, world.LoadGameState())

	eCtx := cardinal.NewWorldContext(world)
	alphaEntity, err := ecs.Create(eCtx, wantOwner)
	assert.NilError(t, err)

	alphaOwner, err := ecs.GetComponent[Owner](eCtx, alphaEntity)
	assert.NilError(t, err)
	assert.Equal(t, *alphaOwner, wantOwner)

	alphaOwner.MyName = "Bob"
	assert.NilError(t, ecs.SetComponent[Owner](eCtx, alphaEntity, alphaOwner))

	newOwner, err := ecs.GetComponent[Owner](eCtx, alphaEntity)
	assert.NilError(t, err)
	assert.Equal(t, newOwner.MyName, "Bob")
}

type Tuple struct {
	A, B int
}

func (Tuple) Name() string {
	return "tuple"
}

func TestCanRemoveEntity(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World

	assert.NilError(t, cardinal.RegisterComponent[Tuple](world))
	assert.NilError(t, world.LoadGameState())

	eCtx := cardinal.NewWorldContext(world)
	entities, err := ecs.CreateMany(eCtx, 2, Tuple{})
	assert.NilError(t, err)

	// Make sure we find exactly 2 entries
	count := 0
	q := cardinal.NewSearch(eCtx, cardinal.Contains(Tuple{}))
	assert.NilError(
		t, q.Each(
			func(id entity.ID) bool {
				_, err = ecs.GetComponent[Tuple](eCtx, id)
				assert.NilError(t, err)
				count++
				return true
			},
		),
	)

	assert.Equal(t, count, 2)
	err = ecs.Remove(eCtx, entities[0])
	assert.NilError(t, err)

	// Now we should only find 1 entity
	count = 0
	assert.NilError(
		t, q.Each(
			func(id entity.ID) bool {
				_, err = ecs.GetComponent[Tuple](eCtx, id)
				assert.NilError(t, err)
				count++
				return true
			},
		),
	)
	assert.Equal(t, count, 1)

	// This entity was Removed, so we shouldn't be able to find it
	_, err = world.GameStateManager().GetComponentTypesForEntity(entities[0])
	assert.Check(t, err != nil)

	// Remove the other entity
	err = ecs.Remove(eCtx, entities[1])
	assert.NilError(t, err)
	count = 0
	assert.NilError(
		t, q.Each(
			func(id entity.ID) bool {
				_, err = ecs.GetComponent[Tuple](eCtx, id)
				assert.NilError(t, err)
				count++
				return true
			},
		),
	)
	assert.Equal(t, count, 0)

	// This entity was Removed, so we shouldn't be able to find it
	_, err = world.GameStateManager().GetComponentTypesForEntity(entities[0])
	assert.Check(t, err != nil)
}

type CountComponent struct {
	Val int
}

func (CountComponent) Name() string {
	return "Count"
}

func TestCanRemoveEntriesDuringCallToEach(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World

	assert.NilError(t, cardinal.RegisterComponent[CountComponent](world))
	assert.NilError(t, world.LoadGameState())

	eCtx := cardinal.NewWorldContext(world)
	_, err := ecs.CreateMany(eCtx, 10, CountComponent{})
	assert.NilError(t, err)

	// Pre-populate all the entities with their own IDs. This will help
	// us keep track of which component belongs to which entity in the case
	// of a problem
	q := cardinal.NewSearch(eCtx, filter.Contains(CountComponent{}))
	assert.NilError(
		t, q.Each(
			func(id entity.ID) bool {
				assert.NilError(t, ecs.SetComponent[CountComponent](eCtx, id, &CountComponent{int(id)}))
				return true
			},
		),
	)

	// Remove the even entries
	itr := 0
	assert.NilError(
		t, q.Each(
			func(id entity.ID) bool {
				if itr%2 == 0 {
					assert.NilError(t, ecs.Remove(eCtx, id))
				}
				itr++
				return true
			},
		),
	)
	// Verify we did this Each the correct number of times
	assert.Equal(t, 10, itr)

	seen := map[int]int{}
	assert.NilError(
		t, q.Each(
			func(id entity.ID) bool {
				c, err := ecs.GetComponent[CountComponent](eCtx, id)
				assert.NilError(t, err)
				seen[c.Val]++
				return true
			},
		),
	)

	// Verify we're left with exactly 5 odd values between 1 and 9
	assert.Equal(t, len(seen), 5)
	for i := 1; i < 10; i += 2 {
		assert.Equal(t, seen[i], 1)
	}
}

func TestAddingAComponentThatAlreadyExistsIsError(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponent](world))
	assert.NilError(t, world.LoadGameState())

	eCtx := cardinal.NewWorldContext(world)
	ent, err := ecs.Create(eCtx, EnergyComponent{})
	assert.NilError(t, err)
	assert.ErrorIs(t, ecs.AddComponentTo[EnergyComponent](eCtx, ent), iterators.ErrComponentAlreadyOnEntity)
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
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, cardinal.RegisterComponent[ReactorEnergy](world))
	assert.NilError(t, cardinal.RegisterComponent[WeaponEnergy](world))
	assert.NilError(t, world.LoadGameState())
	eCtx := cardinal.NewWorldContext(world)
	ent, err := ecs.Create(eCtx, ReactorEnergy{})
	assert.NilError(t, err)

	assert.ErrorIs(t, ecs.RemoveComponentFrom[WeaponEnergy](eCtx, ent), iterators.ErrComponentNotOnEntity)
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
	world := testutils.NewTestFixture(t, nil).World

	assert.NilError(t, cardinal.RegisterComponent[Foo](world))
	assert.NilError(t, cardinal.RegisterComponent[Bar](world))

	assert.NilError(t, world.LoadGameState())

	getArchIDForEntityID := func(id entity.ID) archetype.ID {
		components, err := world.GameStateManager().GetComponentTypesForEntity(id)
		assert.NilError(t, err)
		archID, err := world.GameStateManager().GetArchIDForComponents(components)
		assert.NilError(t, err)
		return archID
	}
	eCtx := cardinal.NewWorldContext(world)
	entity, err := ecs.Create(eCtx, Foo{}, Bar{})
	assert.NilError(t, err)

	archIDBefore := getArchIDForEntityID(entity)

	// The entity should now be in a different archetype
	assert.NilError(t, ecs.RemoveComponentFrom[Foo](eCtx, entity))

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
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))
	assert.NilError(t, cardinal.RegisterComponent[Gamma](world))

	assert.NilError(t, world.LoadGameState())

	eCtx := cardinal.NewWorldContext(world)
	entIDs, err := ecs.CreateMany(eCtx, 3, Alpha{}, Beta{})
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
	alphaQuery := cardinal.NewSearch(eCtx, filter.Contains(Alpha{}))
	err = alphaQuery.Each(countAgain())
	assert.NilError(t, err)
	assert.Equal(t, 3, count)

	// 0 entities have gamma
	gammaQuery := cardinal.NewSearch(eCtx, filter.Contains(Gamma{}))
	err = gammaQuery.Each(countAgain())
	assert.NilError(t, err)
	assert.Equal(t, 0, count)

	assert.NilError(t, ecs.RemoveComponentFrom[Alpha](eCtx, entIDs[0]))

	// alpha has been removed from entity[0], so only 2 entities should now have alpha
	err = alphaQuery.Each(countAgain())
	assert.NilError(t, err)
	assert.Equal(t, 2, count)

	// Add gamma to an entity. Now 1 entity should have gamma.
	assert.NilError(t, ecs.AddComponentTo[Gamma](eCtx, entIDs[1]))
	err = gammaQuery.Each(countAgain())
	assert.NilError(t, err)
	assert.Equal(t, 1, count)

	// Make sure the one ent that has gamma is entIDs[1]
	err = gammaQuery.Each(
		func(id entity.ID) bool {
			assert.Equal(t, id, entIDs[1])
			return true
		},
	)
	assert.NilError(t, err)
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
	world := testutils.NewTestFixture(t, nil).World

	assert.NilError(t, cardinal.RegisterComponent[EnergyComponentAlpha](world))
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponentBeta](world))
	assert.NilError(t, world.LoadGameState())

	eCtx := cardinal.NewWorldContext(world)
	id, err := ecs.Create(eCtx, EnergyComponentAlpha{})
	assert.NilError(t, err)

	err = ecs.SetComponent[EnergyComponentBeta](eCtx, id, &EnergyComponentBeta{100, 200})
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
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, cardinal.RegisterComponent[A](world))
	assert.NilError(t, cardinal.RegisterComponent[B](world))
	assert.NilError(t, cardinal.RegisterComponent[C](world))
	assert.NilError(t, cardinal.RegisterComponent[D](world))

	assert.NilError(t, world.LoadGameState())

	eCtx := cardinal.NewWorldContext(world)
	ab, err := ecs.Create(eCtx, A{}, B{})
	assert.NilError(t, err)
	cd, err := ecs.Create(eCtx, C{}, D{})
	assert.NilError(t, err)
	_, err = ecs.Create(eCtx, B{}, D{})
	assert.NilError(t, err)

	// Only one entity has the components a and b
	abFilter := filter.Contains(A{}, B{})
	q := cardinal.NewSearch(eCtx, abFilter)
	assert.NilError(
		t, q.Each(
			func(id entity.ID) bool {
				assert.Equal(t, id, ab)
				return true
			},
		),
	)
	q = cardinal.NewSearch(eCtx, abFilter)
	num, err := q.Count()
	assert.NilError(t, err)
	assert.Equal(t, num, 1)

	cdFilter := filter.Contains(C{}, D{})
	q = cardinal.NewSearch(eCtx, cdFilter)
	assert.NilError(
		t, q.Each(
			func(id entity.ID) bool {
				assert.Equal(t, id, cd)
				return true
			},
		),
	)
	q = cardinal.NewSearch(eCtx, abFilter)
	num, err = q.Count()
	assert.NilError(t, err)
	assert.Equal(t, num, 1)

	q = cardinal.NewSearch(eCtx, filter.Or(filter.Contains(A{}), filter.Contains(D{})))
	allCount, err := q.Count()
	assert.NilError(t, err)
	assert.Equal(t, allCount, 3)
}

type HealthComponent struct {
	HP int
}

func (HealthComponent) Name() string {
	return "hpComp"
}

func TestUpdateWithPointerType(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, cardinal.RegisterComponent[HealthComponent](world))
	assert.NilError(t, world.LoadGameState())

	eCtx := cardinal.NewWorldContext(world)
	id, err := ecs.Create(eCtx, HealthComponent{})
	assert.NilError(t, err)

	err = ecs.UpdateComponent[HealthComponent](
		eCtx, id, func(h *HealthComponent) *HealthComponent {
			if h == nil {
				h = &HealthComponent{}
			}
			h.HP += 100
			return h
		},
	)
	assert.NilError(t, err)

	hp, err := ecs.GetComponent[HealthComponent](eCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 100, hp.HP)
}

type ValueComponent1 struct {
	Val int
}

func (ValueComponent1) Name() string {
	return "valComp"
}

func TestCanRemoveFirstEntity(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, cardinal.RegisterComponent[ValueComponent1](world))
	assert.NilError(t, world.LoadGameState())

	eCtx := cardinal.NewWorldContext(world)
	ids, err := ecs.CreateMany(eCtx, 3, ValueComponent1{})
	assert.NilError(t, err)
	assert.NilError(t, ecs.SetComponent[ValueComponent1](eCtx, ids[0], &ValueComponent1{99}))
	assert.NilError(t, ecs.SetComponent[ValueComponent1](eCtx, ids[1], &ValueComponent1{100}))
	assert.NilError(t, ecs.SetComponent[ValueComponent1](eCtx, ids[2], &ValueComponent1{101}))

	assert.NilError(t, ecs.Remove(eCtx, ids[0]))

	val, err := ecs.GetComponent[ValueComponent1](eCtx, ids[1])
	assert.NilError(t, err)
	assert.Equal(t, 100, val.Val)

	val, err = ecs.GetComponent[ValueComponent1](eCtx, ids[2])
	assert.NilError(t, err)
	assert.Equal(t, 101, val.Val)
}

type ValueComponent2 struct {
	Val int
}
type OtherComponent struct {
	Val int
}

func (ValueComponent2) Name() string {
	return "valComp"
}

func (OtherComponent) Name() string {
	return "otherComp"
}

func TestCanChangeArchetypeOfFirstEntity(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, cardinal.RegisterComponent[ValueComponent2](world))
	assert.NilError(t, cardinal.RegisterComponent[OtherComponent](world))
	assert.NilError(t, world.LoadGameState())

	eCtx := cardinal.NewWorldContext(world)
	ids, err := ecs.CreateMany(eCtx, 3, ValueComponent2{})
	assert.NilError(t, err)
	assert.NilError(t, ecs.SetComponent[ValueComponent2](eCtx, ids[0], &ValueComponent2{99}))
	assert.NilError(t, ecs.SetComponent[ValueComponent2](eCtx, ids[1], &ValueComponent2{100}))
	assert.NilError(t, ecs.SetComponent[ValueComponent2](eCtx, ids[2], &ValueComponent2{101}))

	assert.NilError(t, ecs.AddComponentTo[OtherComponent](eCtx, ids[0]))

	val, err := ecs.GetComponent[ValueComponent2](eCtx, ids[1])
	assert.NilError(t, err)
	assert.Equal(t, 100, val.Val)

	val, err = ecs.GetComponent[ValueComponent2](eCtx, ids[2])
	assert.NilError(t, err)
	assert.Equal(t, 101, val.Val)
}

func TestEntityCreationAndSetting(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, cardinal.RegisterComponent[ValueComponent2](world))
	assert.NilError(t, cardinal.RegisterComponent[OtherComponent](world))

	eCtx := cardinal.NewWorldContext(world)
	assert.NilError(t, world.LoadGameState())
	ids, err := ecs.CreateMany(eCtx, 300, ValueComponent2{999}, OtherComponent{999})
	assert.NilError(t, err)
	for _, id := range ids {
		x, err := ecs.GetComponent[ValueComponent2](eCtx, id)
		assert.NilError(t, err)
		y, err := ecs.GetComponent[OtherComponent](eCtx, id)
		assert.NilError(t, err)
		assert.Equal(t, x.Val, 999)
		assert.Equal(t, y.Val, 999)
	}
}
