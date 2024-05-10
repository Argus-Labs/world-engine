package cardinal_test

import (
	"errors"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
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

type Pos struct {
	X, Y float64
}

func (Pos) Name() string {
	return "Position"
}

type Vel struct {
	DX, DY float64
}

func (Vel) Name() string {
	return "Velocity"
}

type ReactorEnergy struct {
	Amt int64
	Cap int64
}

func (ReactorEnergy) Name() string {
	return "reactorEnergy"
}

type WeaponEnergy struct {
	Amt int64
	Cap int64
}

func (WeaponEnergy) Name() string {
	return "weaponsEnergy"
}

func UpdateEnergySystem(wCtx cardinal.Context) error {
	var errs []error
	q := cardinal.NewSearch().Entity(filter.Contains(filter.Component[EnergyComponent]()))
	err := q.Each(wCtx,
		func(ent types.EntityID) bool {
			energyPlanet, err := cardinal.GetComponent[EnergyComponent](wCtx, ent)
			if err != nil {
				errs = append(errs, err)
			}
			energyPlanet.Amt += 10 // bs whatever
			err = cardinal.SetComponent[EnergyComponent](wCtx, ent, energyPlanet)
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

func TestECS(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponent](world))
	assert.NilError(t, cardinal.RegisterComponent[OwnableComponent](world))

	// cardinal.Create a bunch of planets!
	numPlanets := 5
	wCtx := cardinal.NewWorldContext(world)

	err := cardinal.RegisterSystems(world, UpdateEnergySystem)
	assert.NilError(t, err)

	tf.StartWorld()

	numEnergyOnly := 10
	_, err = cardinal.CreateMany(wCtx, numEnergyOnly, EnergyComponent{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(wCtx, numPlanets, EnergyComponent{}, OwnableComponent{})
	assert.NilError(t, err)

	tf.DoTick()
	query := cardinal.NewSearch().Entity(filter.Contains(filter.Component[EnergyComponent]()))
	err = query.Each(wCtx,
		func(id types.EntityID) bool {
			energyPlanet, err := cardinal.GetComponent[EnergyComponent](wCtx, id)
			assert.NilError(t, err)
			assert.Equal(t, int64(10), energyPlanet.Amt)
			return true
		},
	)
	assert.NilError(t, err)

	q := cardinal.Or(cardinal.NewSearch().Entity(
		filter.Contains(filter.Component[EnergyComponent]())), cardinal.NewSearch().Entity(
		filter.Contains(filter.Component[OwnableComponent]())))
	amt, err := q.Count(wCtx)
	assert.NilError(t, err)
	assert.Equal(t, numPlanets+numEnergyOnly, amt)
	comp, err := world.GetComponentByName("EnergyComponent")
	assert.NilError(t, err)
	var energyComponent EnergyComponent
	assert.Equal(t, comp.Name(), energyComponent.Name())
}

func TestVelocitySimulation(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	// These components are a mix of concrete types and pointer types to make sure they both work
	assert.NilError(t, cardinal.RegisterComponent[Pos](world))
	assert.NilError(t, cardinal.RegisterComponent[Vel](world))

	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	shipID, err := cardinal.Create(wCtx, Pos{}, Vel{})
	assert.NilError(t, err)
	assert.NilError(t, cardinal.SetComponent[Pos](wCtx, shipID, &Pos{1, 2}))
	assert.NilError(t, cardinal.SetComponent[Vel](wCtx, shipID, &Vel{3, 4}))
	wantPos := Pos{4, 6}

	velocityQuery := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Vel]()))
	err = velocityQuery.Each(wCtx,
		func(id types.EntityID) bool {
			vel, err := cardinal.GetComponent[Vel](wCtx, id)
			assert.NilError(t, err)
			pos, err := cardinal.GetComponent[Pos](wCtx, id)
			assert.NilError(t, err)
			newPos := Pos{pos.X + vel.DX, pos.Y + vel.DY}
			assert.NilError(t, cardinal.SetComponent[Pos](wCtx, id, &newPos))
			return true
		},
	)
	assert.NilError(t, err)

	finalPos, err := cardinal.GetComponent[Pos](wCtx, shipID)
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	wantOwner := Owner{"Jeff"}

	assert.NilError(t, cardinal.RegisterComponent[Owner](world))

	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	alphaEntity, err := cardinal.Create(wCtx, wantOwner)
	assert.NilError(t, err)

	alphaOwner, err := cardinal.GetComponent[Owner](wCtx, alphaEntity)
	assert.NilError(t, err)
	assert.Equal(t, *alphaOwner, wantOwner)

	alphaOwner.MyName = "Bob"
	assert.NilError(t, cardinal.SetComponent[Owner](wCtx, alphaEntity, alphaOwner))

	newOwner, err := cardinal.GetComponent[Owner](wCtx, alphaEntity)
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	assert.NilError(t, cardinal.RegisterComponent[Tuple](world))

	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	entities, err := cardinal.CreateMany(wCtx, 2, Tuple{})
	assert.NilError(t, err)

	// Make sure we find exactly 2 entries
	count := 0
	q := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Tuple]()))
	assert.NilError(
		t, q.Each(wCtx,
			func(id types.EntityID) bool {
				_, err = cardinal.GetComponent[Tuple](wCtx, id)
				assert.NilError(t, err)
				count++
				return true
			},
		),
	)

	assert.Equal(t, count, 2)
	err = cardinal.Remove(wCtx, entities[0])
	assert.NilError(t, err)

	// Now we should only find 1 entity
	count = 0
	assert.NilError(
		t, q.Each(wCtx,
			func(id types.EntityID) bool {
				_, err = cardinal.GetComponent[Tuple](wCtx, id)
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
	err = cardinal.Remove(wCtx, entities[1])
	assert.NilError(t, err)
	count = 0
	assert.NilError(
		t, q.Each(wCtx,
			func(id types.EntityID) bool {
				_, err = cardinal.GetComponent[Tuple](wCtx, id)
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	assert.NilError(t, cardinal.RegisterComponent[CountComponent](world))
	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	_, err := cardinal.CreateMany(wCtx, 10, CountComponent{})
	assert.NilError(t, err)

	// Pre-populate all the entities with their own IDs. This will help
	// us keep track of which component belongs to which entity in the case
	// of a problem
	q := cardinal.NewSearch().Entity(filter.Contains(filter.Component[CountComponent]()))
	assert.NilError(
		t, q.Each(wCtx,
			func(id types.EntityID) bool {
				assert.NilError(t, cardinal.SetComponent[CountComponent](wCtx, id, &CountComponent{int(id)}))
				return true
			},
		),
	)

	// Remove the even entries
	itr := 0
	assert.NilError(
		t, q.Each(wCtx,
			func(id types.EntityID) bool {
				if itr%2 == 0 {
					assert.NilError(t, cardinal.Remove(wCtx, id))
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
		t, q.Each(wCtx,
			func(id types.EntityID) bool {
				c, err := cardinal.GetComponent[CountComponent](wCtx, id)
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponent](world))
	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	ent, err := cardinal.Create(wCtx, EnergyComponent{})
	assert.NilError(t, err)
	assert.ErrorIs(t, cardinal.AddComponentTo[EnergyComponent](wCtx, ent), iterators.ErrComponentAlreadyOnEntity)
}

func TestRemovingAMissingComponentIsError(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[ReactorEnergy](world))
	assert.NilError(t, cardinal.RegisterComponent[WeaponEnergy](world))
	tf.StartWorld()
	wCtx := cardinal.NewWorldContext(world)
	ent, err := cardinal.Create(wCtx, ReactorEnergy{})
	assert.NilError(t, err)

	assert.ErrorIs(t, cardinal.RemoveComponentFrom[WeaponEnergy](wCtx, ent), iterators.ErrComponentNotOnEntity)
}

func TestVerifyAutomaticCreationOfArchetypesWorks(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	assert.NilError(t, cardinal.RegisterComponent[Foo](world))
	assert.NilError(t, cardinal.RegisterComponent[Bar](world))

	tf.StartWorld()

	getArchIDForEntityID := func(id types.EntityID) types.ArchetypeID {
		components, err := world.GameStateManager().GetComponentTypesForEntity(id)
		assert.NilError(t, err)
		archID, err := world.GameStateManager().GetArchIDForComponents(components)
		assert.NilError(t, err)
		return archID
	}
	wCtx := cardinal.NewWorldContext(world)
	entity, err := cardinal.Create(wCtx, Foo{}, Bar{})
	assert.NilError(t, err)

	archIDBefore := getArchIDForEntityID(entity)

	// The entity should now be in a different archetype
	assert.NilError(t, cardinal.RemoveComponentFrom[Foo](wCtx, entity))

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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))
	assert.NilError(t, cardinal.RegisterComponent[Gamma](world))

	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	entIDs, err := cardinal.CreateMany(wCtx, 3, Alpha{}, Beta{})
	assert.NilError(t, err)

	// count and countAgain are helpers that simplify the counting of how many
	// entities have a particular component.
	var count int
	countAgain := func() func(ent types.EntityID) bool {
		count = 0
		return func(types.EntityID) bool {
			count++
			return true
		}
	}
	// 3 entities have alpha
	alphaQuery := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Alpha]()))
	err = alphaQuery.Each(wCtx, countAgain())
	assert.NilError(t, err)
	assert.Equal(t, 3, count)

	// 0 entities have gamma
	gammaQuery := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Gamma]()))
	err = gammaQuery.Each(wCtx, countAgain())
	assert.NilError(t, err)
	assert.Equal(t, 0, count)

	assert.NilError(t, cardinal.RemoveComponentFrom[Alpha](wCtx, entIDs[0]))

	// alpha has been removed from entity[0], so only 2 entities should now have alpha
	err = alphaQuery.Each(wCtx, countAgain())
	assert.NilError(t, err)
	assert.Equal(t, 2, count)

	// Add gamma to an entity. Now 1 entity should have gamma.
	assert.NilError(t, cardinal.AddComponentTo[Gamma](wCtx, entIDs[1]))
	err = gammaQuery.Each(wCtx, countAgain())
	assert.NilError(t, err)
	assert.Equal(t, 1, count)

	// Make sure the one ent that has gamma is entIDs[1]
	err = gammaQuery.Each(wCtx,
		func(id types.EntityID) bool {
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	assert.NilError(t, cardinal.RegisterComponent[EnergyComponentAlpha](world))
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponentBeta](world))
	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	id, err := cardinal.Create(wCtx, EnergyComponentAlpha{})
	assert.NilError(t, err)

	err = cardinal.SetComponent[EnergyComponentBeta](wCtx, id, &EnergyComponentBeta{100, 200})
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[A](world))
	assert.NilError(t, cardinal.RegisterComponent[B](world))
	assert.NilError(t, cardinal.RegisterComponent[C](world))
	assert.NilError(t, cardinal.RegisterComponent[D](world))

	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	ab, err := cardinal.Create(wCtx, A{}, B{})
	assert.NilError(t, err)
	cd, err := cardinal.Create(wCtx, C{}, D{})
	assert.NilError(t, err)
	_, err = cardinal.Create(wCtx, B{}, D{})
	assert.NilError(t, err)

	// Only one entity has the components a and b
	q := cardinal.NewSearch().Entity(filter.Contains(filter.Component[A](), filter.Component[B]()))
	assert.NilError(
		t, q.Each(wCtx,
			func(id types.EntityID) bool {
				assert.Equal(t, id, ab)
				return true
			},
		),
	)
	q = cardinal.NewSearch().Entity(filter.Contains(filter.Component[A](), filter.Component[B]()))
	num, err := q.Count(wCtx)
	assert.NilError(t, err)
	assert.Equal(t, num, 1)

	q = cardinal.NewSearch().Entity(filter.Contains(filter.Component[C](), filter.Component[D]()))
	assert.NilError(
		t, q.Each(wCtx,
			func(id types.EntityID) bool {
				assert.Equal(t, id, cd)
				return true
			},
		),
	)
	q = cardinal.NewSearch().Entity(filter.Contains(filter.Component[A](), filter.Component[B]()))
	num, err = q.Count(wCtx)
	assert.NilError(t, err)
	assert.Equal(t, num, 1)

	searchable := cardinal.Or(cardinal.NewSearch().Entity(
		filter.Contains(filter.Component[A]())),
		cardinal.NewSearch().Entity(filter.Contains(filter.Component[D]())))
	allCount, err := searchable.Count(wCtx)
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[HealthComponent](world))
	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	id, err := cardinal.Create(wCtx, HealthComponent{})
	assert.NilError(t, err)

	err = cardinal.UpdateComponent[HealthComponent](
		wCtx, id, func(h *HealthComponent) *HealthComponent {
			if h == nil {
				h = &HealthComponent{}
			}
			h.HP += 100
			return h
		},
	)
	assert.NilError(t, err)

	hp, err := cardinal.GetComponent[HealthComponent](wCtx, id)
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[ValueComponent1](world))
	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	ids, err := cardinal.CreateMany(wCtx, 3, ValueComponent1{})
	assert.NilError(t, err)
	assert.NilError(t, cardinal.SetComponent[ValueComponent1](wCtx, ids[0], &ValueComponent1{99}))
	assert.NilError(t, cardinal.SetComponent[ValueComponent1](wCtx, ids[1], &ValueComponent1{100}))
	assert.NilError(t, cardinal.SetComponent[ValueComponent1](wCtx, ids[2], &ValueComponent1{101}))

	assert.NilError(t, cardinal.Remove(wCtx, ids[0]))

	val, err := cardinal.GetComponent[ValueComponent1](wCtx, ids[1])
	assert.NilError(t, err)
	assert.Equal(t, 100, val.Val)

	val, err = cardinal.GetComponent[ValueComponent1](wCtx, ids[2])
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[ValueComponent2](world))
	assert.NilError(t, cardinal.RegisterComponent[OtherComponent](world))
	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	ids, err := cardinal.CreateMany(wCtx, 3, ValueComponent2{})
	assert.NilError(t, err)
	assert.NilError(t, cardinal.SetComponent[ValueComponent2](wCtx, ids[0], &ValueComponent2{99}))
	assert.NilError(t, cardinal.SetComponent[ValueComponent2](wCtx, ids[1], &ValueComponent2{100}))
	assert.NilError(t, cardinal.SetComponent[ValueComponent2](wCtx, ids[2], &ValueComponent2{101}))

	assert.NilError(t, cardinal.AddComponentTo[OtherComponent](wCtx, ids[0]))

	val, err := cardinal.GetComponent[ValueComponent2](wCtx, ids[1])
	assert.NilError(t, err)
	assert.Equal(t, 100, val.Val)

	val, err = cardinal.GetComponent[ValueComponent2](wCtx, ids[2])
	assert.NilError(t, err)
	assert.Equal(t, 101, val.Val)
}

func TestEntityCreationAndSetting(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[ValueComponent2](world))
	assert.NilError(t, cardinal.RegisterComponent[OtherComponent](world))

	wCtx := cardinal.NewWorldContext(world)
	tf.StartWorld()
	ids, err := cardinal.CreateMany(wCtx, 300, ValueComponent2{999}, OtherComponent{999})
	assert.NilError(t, err)
	for _, id := range ids {
		x, err := cardinal.GetComponent[ValueComponent2](wCtx, id)
		assert.NilError(t, err)
		y, err := cardinal.GetComponent[OtherComponent](wCtx, id)
		assert.NilError(t, err)
		assert.Equal(t, x.Val, 999)
		assert.Equal(t, y.Val, 999)
	}
}
