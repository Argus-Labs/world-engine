package ecs_test

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/alicebob/miniredis/v2"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

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

func TestErrorWhenSavedArchetypesDoNotMatchComponentTypes(t *testing.T) {
	// This redisStore will be used to create multiple worlds to ensure state is consistent across the worlds.
	redisStore := miniredis.RunT(t)

	oneWorld := testutil.InitWorldWithRedis(t, redisStore)
	oneAlphaNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, oneWorld.RegisterComponents(oneAlphaNum))
	assert.NilError(t, oneWorld.LoadGameState())

	_, err := oneWorld.Create(oneAlphaNum)
	assert.NilError(t, err)

	assert.NilError(t, oneWorld.Tick(context.Background()))

	// Too few components registered
	twoWorld := testutil.InitWorldWithRedis(t, redisStore)
	assert.NilError(t, twoWorld.RegisterComponents())
	err = twoWorld.LoadGameState()
	assert.ErrorContains(t, err, storage.ErrorComponentMismatchWithSavedState.Error())

	// It's ok to register extra components.
	threeWorld := testutil.InitWorldWithRedis(t, redisStore)
	threeAlphaNum := ecs.NewComponentType[NumberComponent]()
	threeBetaNum := ecs.NewComponentType[NumberComponent]()
	threeBetaNum.SetName("threeBetaNum")
	threeAlphaNum.SetName("threeAlphaNum")
	assert.NilError(t, threeWorld.RegisterComponents(threeAlphaNum, threeBetaNum))
	assert.NilError(t, threeWorld.LoadGameState())

	// Just the right number of components registered
	fourWorld := testutil.InitWorldWithRedis(t, redisStore)
	fourAlphaNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, fourWorld.RegisterComponents(fourAlphaNum))
	assert.NilError(t, fourWorld.LoadGameState())
}

func TestArchetypeIDIsConsistentAfterSaveAndLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)
	oneWorld := testutil.InitWorldWithRedis(t, redisStore)
	oneNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, oneWorld.RegisterComponents(oneNum))
	assert.NilError(t, oneWorld.LoadGameState())

	_, err := oneWorld.Create(oneNum)
	assert.NilError(t, err)

	wantID := oneWorld.GetArchetypeForComponents(comps(oneNum))
	wantLayout := oneWorld.Archetype(wantID).Layout()
	assert.Equal(t, 1, len(wantLayout.Components()))
	assert.Check(t, wantLayout.HasComponent(oneNum))

	assert.NilError(t, oneWorld.Tick(context.Background()))

	// Make a second instance of the world using the same storage.
	twoWorld := testutil.InitWorldWithRedis(t, redisStore)
	twoNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, twoWorld.RegisterComponents(twoNum))
	assert.NilError(t, twoWorld.LoadGameState())

	gotID := twoWorld.GetArchetypeForComponents(comps(twoNum))
	gotLayout := twoWorld.Archetype(gotID).Layout()
	assert.Equal(t, 1, len(gotLayout.Components()))
	assert.Check(t, gotLayout.HasComponent(twoNum))

	// Archetype indices should be the same across save/load cycles
	assert.Equal(t, wantID, gotID)
}

func TestCanRecoverArchetypeInformationAfterLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)

	oneWorld := testutil.InitWorldWithRedis(t, redisStore)
	oneAlphaNum := ecs.NewComponentType[NumberComponent]()
	oneBetaNum := ecs.NewComponentType[NumberComponent]()
	oneAlphaNum.SetName("oneAlphaNum")
	oneBetaNum.SetName("OneBetaNum")
	assert.NilError(t, oneWorld.RegisterComponents(oneAlphaNum, oneBetaNum))
	assert.NilError(t, oneWorld.LoadGameState())

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
	oneJustAlphaArchID := oneWorld.GetArchetypeForComponents(comps(oneAlphaNum))
	oneJustBetaArchID := oneWorld.GetArchetypeForComponents(comps(oneBetaNum))
	oneBothArchID := oneWorld.GetArchetypeForComponents(comps(oneAlphaNum, oneBetaNum))
	// These archetype indices should be preserved between a state save/load

	assert.NilError(t, oneWorld.Tick(context.Background()))

	// Create a brand new world, but use the original redis store. We should be able to load
	// the game state from the redis store (including archetype indices).
	twoWorld := testutil.InitWorldWithRedis(t, redisStore)
	twoAlphaNum := ecs.NewComponentType[NumberComponent]()
	twoBetaNum := ecs.NewComponentType[NumberComponent]()
	// The ordering of registering these components is important. It must match the ordering above.
	twoAlphaNum.SetName("towAlphaNum")
	twoBetaNum.SetName("towBetaNum")
	assert.NilError(t, twoWorld.RegisterComponents(twoAlphaNum, twoBetaNum))
	assert.NilError(t, twoWorld.LoadGameState())

	// Don't create any entities like above; they should already exist

	// The order that we FETCH archetypes shouldn't matter, so this order is intentionally
	// different from the setup step
	twoBothArchID := oneWorld.GetArchetypeForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.Equal(t, oneBothArchID, twoBothArchID)
	twoJustAlphaArchID := oneWorld.GetArchetypeForComponents(comps(oneAlphaNum))
	assert.Equal(t, oneJustAlphaArchID, twoJustAlphaArchID)
	twoJustBetaArchID := oneWorld.GetArchetypeForComponents(comps(oneBetaNum))
	assert.Equal(t, oneJustBetaArchID, twoJustBetaArchID)

	// Save and load again to make sure the "two" world correctly saves its state even though
	// it never created any entities
	assert.NilError(t, twoWorld.Tick(context.Background()))

	threeWorld := testutil.InitWorldWithRedis(t, redisStore)
	threeAlphaNum := ecs.NewComponentType[NumberComponent]()
	threeBetaNum := ecs.NewComponentType[NumberComponent]()
	// Again, the ordering of registering these components is important. It must match the ordering above
	threeAlphaNum.SetName("threeAlphaNum")
	threeBetaNum.SetName("threeBetaNum")
	assert.NilError(t, threeWorld.RegisterComponents(threeAlphaNum, threeBetaNum))
	assert.NilError(t, threeWorld.LoadGameState())

	// And again, the loading of archetypes is intentionally different from the above two steps
	threeJustBetaArchID := oneWorld.GetArchetypeForComponents(comps(oneBetaNum))
	assert.Equal(t, oneJustBetaArchID, threeJustBetaArchID)
	threeBothArchID := oneWorld.GetArchetypeForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.Equal(t, oneBothArchID, threeBothArchID)
	threeJustAlphaArchID := oneWorld.GetArchetypeForComponents(comps(oneAlphaNum))
	assert.Equal(t, oneJustAlphaArchID, threeJustAlphaArchID)
}

func TestCanReloadState(t *testing.T) {
	redisStore := miniredis.RunT(t)
	alphaWorld := testutil.InitWorldWithRedis(t, redisStore)
	oneAlphaNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, alphaWorld.RegisterComponents(oneAlphaNum))

	_, err := alphaWorld.CreateMany(10, oneAlphaNum)
	assert.NilError(t, err)
	alphaWorld.AddSystem(func(w *ecs.World, queue *ecs.TransactionQueue, _ *ecs.Logger) error {
		oneAlphaNum.Each(w, func(id storage.EntityID) bool {
			err := oneAlphaNum.Set(w, id, NumberComponent{int(id)})
			assert.Check(t, err == nil)
			return true
		})
		return nil
	})
	assert.NilError(t, alphaWorld.LoadGameState())

	// Start a tick with executes the above system which initializes the number components.
	assert.NilError(t, alphaWorld.Tick(context.Background()))

	// Make a new world, using the original redis DB that (hopefully) has our data
	betaWorld := testutil.InitWorldWithRedis(t, redisStore)
	oneBetaNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, betaWorld.RegisterComponents(oneBetaNum))
	assert.NilError(t, betaWorld.LoadGameState())

	count := 0
	oneBetaNum.Each(betaWorld, func(id storage.EntityID) bool {
		count++
		num, err := oneBetaNum.Get(betaWorld, id)
		assert.NilError(t, err)
		assert.Equal(t, int(id), num.Num)
		return true
	})
	// Make sure we actually have 10 entities
	assert.Equal(t, 10, count)
}

func TestWorldTickAndHistoryTickMatch(t *testing.T) {
	redisStore := miniredis.RunT(t)
	ctx := context.Background()

	// Ensure that across multiple reloads, getting the transaction receipts for a tick
	// that is still in the tx receipt history window will not return any errors.
	for reload := 0; reload < 5; reload++ {
		world := testutil.InitWorldWithRedis(t, redisStore)
		assert.NilError(t, world.LoadGameState())
		relevantTick := world.CurrentTick()
		for i := 0; i < 5; i++ {
			assert.NilError(t, world.Tick(ctx))
		}
		// Ignore the actual receipts (they will be empty). Just make sure the tick we're asking
		// for isn't considered too far in the future.
		_, err := world.GetTransactionReceiptsForTick(relevantTick)
		assert.NilError(t, err, "error in reload %d", reload)
	}
}

func TestCanFindTransactionsAfterReloadingWorld(t *testing.T) {
	type Msg struct{}
	type Result struct{}
	someTx := ecs.NewTransactionType[Msg, Result]("some-tx")
	redisStore := miniredis.RunT(t)
	ctx := context.Background()

	// Ensure that across multiple reloads we can queue up transactions, execute those transactions
	// in a tick, and then find those transactions in the tx receipt history.
	for reload := 0; reload < 5; reload++ {
		world := testutil.InitWorldWithRedis(t, redisStore)
		assert.NilError(t, world.RegisterTransactions(someTx))
		world.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue, logger *ecs.Logger) error {
			for _, tx := range someTx.In(queue) {
				someTx.SetResult(world, tx.TxHash, Result{})
			}
			return nil
		})
		assert.NilError(t, world.LoadGameState())

		relevantTick := world.CurrentTick()
		for i := 0; i < 3; i++ {
			_ = someTx.AddToQueue(world, Msg{}, testutil.UniqueSignature(t))
		}

		for i := 0; i < 5; i++ {
			assert.NilError(t, world.Tick(ctx))
		}

		receipts, err := world.GetTransactionReceiptsForTick(relevantTick)
		assert.NilError(t, err)
		assert.Equal(t, 3, len(receipts))
	}
}
