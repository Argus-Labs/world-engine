package tests

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"gotest.tools/v3/assert"
)

// initWorldWithRedis sets up an ecs.World using the given redis DB. ecs.NewECSWorldForTest is not used
// because these tests need to reuse the incoming *miniredis.Miniredis
func initWorldWithRedis(t *testing.T, s *miniredis.Miniredis) *ecs.World {
	rs := storage.NewRedisStorage(storage.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, "in-memory-world")
	worldStorage := storage.NewWorldStorage(
		storage.Components{Store: &rs, ComponentIndices: &rs},
		&rs,
		storage.NewArchetypeComponentIndex(),
		storage.NewArchetypeAccessor(),
		&rs,
		&rs,
		&rs)
	w, err := ecs.NewWorld(worldStorage)
	assert.NilError(t, err)
	return w
}

// comps reduces the typing needed to create a slice of IComponentTypes
// []component.IComponentType{a, b, c} becomes:
// comps(a, b, c)
func comps(cs ...component.IComponentType) []component.IComponentType {
	return cs
}

type NumberComponent struct {
	Num int
}

func TestComponentsCanOnlyBeRegisteredOnce(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.RegisterComponents())
	assert.ErrorIs(t, world.RegisterComponents(), ecs.ErrorComponentRegistrationMustHappenOnce)
}

func TestErrorWhenSavedStateDoesNotMatchComponentTypes(t *testing.T) {
	// This redisStore will be used to create multiple worlds to ensure state is consistent across the worlds.
	redisStore := miniredis.RunT(t)

	oneWorld := initWorldWithRedis(t, redisStore)
	oneAlphaNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, oneWorld.RegisterComponents(oneAlphaNum))

	_, err := oneWorld.Create(oneAlphaNum)
	assert.NilError(t, err)

	assert.NilError(t, oneWorld.SaveGameState())

	// Too few components registered
	twoWorld := initWorldWithRedis(t, redisStore)
	err = twoWorld.RegisterComponents()
	assert.ErrorContains(t, err, storage.ErrorComponentMismatchWithSavedState.Error())

	// It's ok to register extra components.
	threeWorld := initWorldWithRedis(t, redisStore)
	threeAlphaNum := ecs.NewComponentType[NumberComponent]()
	threeBetaNum := ecs.NewComponentType[NumberComponent]()
	err = threeWorld.RegisterComponents(threeAlphaNum, threeBetaNum)
	assert.NilError(t, err)

	// Just the right number of components registered
	fourWorld := initWorldWithRedis(t, redisStore)
	fourAlphaNum := ecs.NewComponentType[NumberComponent]()
	err = fourWorld.RegisterComponents(fourAlphaNum)
	assert.NilError(t, err)
}

func TestArchetypeIndexIsConsistentAfterSaveAndLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)
	oneWorld := initWorldWithRedis(t, redisStore)
	oneNum := ecs.NewComponentType[NumberComponent]()
	oneWorld.RegisterComponents(oneNum)

	_, err := oneWorld.Create(oneNum)
	assert.NilError(t, err)

	wantIndex := oneWorld.GetArchetypeForComponents(comps(oneNum))
	wantLayout := oneWorld.Archetype(wantIndex).Layout()
	assert.Equal(t, 1, len(wantLayout.Components()))
	assert.Check(t, wantLayout.HasComponent(oneNum))

	assert.NilError(t, oneWorld.SaveGameState())

	// Make a second instance of the world using the same storage.
	twoWorld := initWorldWithRedis(t, redisStore)
	twoNum := ecs.NewComponentType[NumberComponent]()
	twoWorld.RegisterComponents(twoNum)

	gotIndex := twoWorld.GetArchetypeForComponents(comps(twoNum))
	gotLayout := twoWorld.Archetype(gotIndex).Layout()
	assert.Equal(t, 1, len(gotLayout.Components()))
	assert.Check(t, gotLayout.HasComponent(twoNum))

	// Archetype indices should be the same across save/load cycles
	assert.Equal(t, wantIndex, gotIndex)
}

func TestCanRecoverArchetypeInformationAfterLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)

	oneWorld := initWorldWithRedis(t, redisStore)
	oneAlphaNum := ecs.NewComponentType[NumberComponent]()
	oneBetaNum := ecs.NewComponentType[NumberComponent]()
	oneWorld.RegisterComponents(oneAlphaNum, oneBetaNum)
	_, err := oneWorld.Create(oneAlphaNum)
	assert.NilError(t, err)
	_, err = oneWorld.Create(oneBetaNum)
	assert.NilError(t, err)
	_, err = oneWorld.Create(oneAlphaNum, oneBetaNum)
	assert.NilError(t, err)
	// At this point 3 archetypes exist:
	// oneAlphaNum
	// oneBetaNum
	// oneAlphaNum, oneBetaNum
	oneJustAlphaArchIndex := oneWorld.GetArchetypeForComponents(comps(oneAlphaNum))
	oneJustBetaArchIndex := oneWorld.GetArchetypeForComponents(comps(oneBetaNum))
	oneBothArchIndex := oneWorld.GetArchetypeForComponents(comps(oneAlphaNum, oneBetaNum))
	// These archetype indices should be preserved between a state save/load

	assert.NilError(t, oneWorld.SaveGameState())

	// Create a brand new world, but use the original redis store. We should be able to load
	// the game state from the redis store (including archetype indices).
	twoWorld := initWorldWithRedis(t, redisStore)
	twoAlphaNum := ecs.NewComponentType[NumberComponent]()
	twoBetaNum := ecs.NewComponentType[NumberComponent]()
	// The ordering of registering these components is important. It must match the ordering above.
	twoWorld.RegisterComponents(twoAlphaNum, twoBetaNum)

	// Don't create any entities like above; they should already exist

	// The order that we FETCH archetypes shouldn't matter, so this order is intentionally
	// different from the setup step
	twoBothArchIndex := oneWorld.GetArchetypeForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.Equal(t, oneBothArchIndex, twoBothArchIndex)
	twoJustAlphaArchIndex := oneWorld.GetArchetypeForComponents(comps(oneAlphaNum))
	assert.Equal(t, oneJustAlphaArchIndex, twoJustAlphaArchIndex)
	twoJustBetaArchIndex := oneWorld.GetArchetypeForComponents(comps(oneBetaNum))
	assert.Equal(t, oneJustBetaArchIndex, twoJustBetaArchIndex)

	// Save and load again to make sure the "two" world correctly saves its state even though
	// it never created any entities
	assert.NilError(t, twoWorld.SaveGameState())

	threeWorld := initWorldWithRedis(t, redisStore)
	threeAlphaNum := ecs.NewComponentType[NumberComponent]()
	threeBetaNum := ecs.NewComponentType[NumberComponent]()
	// Again, the ordering of registering these components is important. It must match the ordering above
	threeWorld.RegisterComponents(threeAlphaNum, threeBetaNum)

	// And again, the loading of archetypes is intentionally different from the above two steps
	threeJustBetaArchIndex := oneWorld.GetArchetypeForComponents(comps(oneBetaNum))
	assert.Equal(t, oneJustBetaArchIndex, threeJustBetaArchIndex)
	threeBothArchIndex := oneWorld.GetArchetypeForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.Equal(t, oneBothArchIndex, threeBothArchIndex)
	threeJustAlphaArchIndex := oneWorld.GetArchetypeForComponents(comps(oneAlphaNum))
	assert.Equal(t, oneJustAlphaArchIndex, threeJustAlphaArchIndex)
}

func TestCanReloadState(t *testing.T) {
	redisStore := miniredis.RunT(t)
	alphaWorld := initWorldWithRedis(t, redisStore)
	oneAlphaNum := ecs.NewComponentType[NumberComponent]()
	alphaWorld.RegisterComponents(oneAlphaNum)

	_, err := alphaWorld.CreateMany(10, oneAlphaNum)
	assert.NilError(t, err)
	alphaWorld.AddSystem(func(w *ecs.World, queue *ecs.TransactionQueue) {
		oneAlphaNum.Each(w, func(id storage.EntityID) {
			err := oneAlphaNum.Set(w, id, &NumberComponent{int(id)})
			assert.Check(t, err == nil)
		})
	})

	// Start a tick with executes the above system which initializes the number components.
	alphaWorld.Tick()
	assert.NilError(t, alphaWorld.SaveGameState())

	// Make a new world, using the original redis DB that (hopefully) has our data
	betaWorld := initWorldWithRedis(t, redisStore)
	oneBetaNum := ecs.NewComponentType[NumberComponent]()
	betaWorld.RegisterComponents(oneBetaNum)
	count := 0
	oneBetaNum.Each(betaWorld, func(id storage.EntityID) {
		count++
		num, err := oneBetaNum.Get(betaWorld, id)
		assert.NilError(t, err)
		assert.Equal(t, int(id), num.Num)
	})
	// Make sure we actually have 10 entities
	assert.Equal(t, 10, count)
}
