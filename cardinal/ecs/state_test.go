package ecs_test

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/alicebob/miniredis/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/component/metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/ecstestutils"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

// comps reduces the typing needed to create a slice of IComponentTypes
// []component.ComponentMetadata{a, b, c} becomes:
// comps(a, b, c).
func comps(cs ...metadata.ComponentMetadata) []metadata.ComponentMetadata {
	return cs
}

type NumberComponent struct {
	Num int
}

func (NumberComponent) Name() string {
	return "oneAlphaNum"
}

type OneAlphaNum struct{}

func (OneAlphaNum) Name() string { return "oneAlphaNum" }

type TwoAlphaNum struct{}

func (TwoAlphaNum) Name() string { return "twoAlphaNum" }

type TwoBetaNum struct{}

func (TwoBetaNum) Name() string { return "twoBetaNum" }

type ThreeAlphaNum struct{}

func (ThreeAlphaNum) Name() string { return "threeAlphaNum" }

type ThreeBetaNum struct{}

func (ThreeBetaNum) Name() string { return "threeBetaNum" }

type FoundAlphaNum struct{}

func (FoundAlphaNum) Name() string { return "foundAlphaNum" }

func TestErrorWhenSavedArchetypesDoNotMatchComponentTypes(t *testing.T) {
	// This redisStore will be used to create multiple worlds to ensure state is consistent across the worlds.
	redisStore := miniredis.RunT(t)

	oneWorld := ecstestutils.InitWorldWithRedis(t, redisStore)
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[OneAlphaNum](oneWorld))
	testutils.AssertNilErrorWithTrace(t, oneWorld.LoadGameState())

	_, err := component.Create(ecs.NewWorldContext(oneWorld), OneAlphaNum{})
	testutils.AssertNilErrorWithTrace(t, err)

	testutils.AssertNilErrorWithTrace(t, oneWorld.Tick(context.Background()))

	// Too few components registered
	twoWorld := ecstestutils.InitWorldWithRedis(t, redisStore)
	err = twoWorld.LoadGameState()
	assert.ErrorContains(t, err, storage.ErrComponentMismatchWithSavedState.Error())

	// It's ok to register extra components.
	threeWorld := ecstestutils.InitWorldWithRedis(t, redisStore)
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[ThreeAlphaNum](threeWorld))
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[ThreeBetaNum](threeWorld))
	testutils.AssertNilErrorWithTrace(t, threeWorld.LoadGameState())

	// Just the right number of components registered
	fourWorld := ecstestutils.InitWorldWithRedis(t, redisStore)
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[FoundAlphaNum](fourWorld))
	testutils.AssertNilErrorWithTrace(t, fourWorld.LoadGameState())
}

func TestArchetypeIDIsConsistentAfterSaveAndLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)
	oneWorld := ecstestutils.InitWorldWithRedis(t, redisStore)
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[NumberComponent](oneWorld))
	testutils.AssertNilErrorWithTrace(t, oneWorld.LoadGameState())

	_, err := component.Create(ecs.NewWorldContext(oneWorld), NumberComponent{})
	testutils.AssertNilErrorWithTrace(t, err)
	oneNum, err := oneWorld.GetComponentByName(NumberComponent{}.Name())
	testutils.AssertNilErrorWithTrace(t, err)
	wantID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneNum))
	testutils.AssertNilErrorWithTrace(t, err)
	wantComps := oneWorld.StoreManager().GetComponentTypesForArchID(wantID)
	assert.Equal(t, 1, len(wantComps))
	assert.Check(t, filter.MatchComponentMetaData(wantComps, oneNum))

	testutils.AssertNilErrorWithTrace(t, oneWorld.Tick(context.Background()))

	// Make a second instance of the world using the same storage.
	twoWorld := ecstestutils.InitWorldWithRedis(t, redisStore)
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[NumberComponent](twoWorld))
	testutils.AssertNilErrorWithTrace(t, twoWorld.LoadGameState())
	twoNum, err := twoWorld.GetComponentByName(NumberComponent{}.Name())
	testutils.AssertNilErrorWithTrace(t, err)
	gotID, err := twoWorld.StoreManager().GetArchIDForComponents(comps(twoNum))
	testutils.AssertNilErrorWithTrace(t, err)
	gotComps := twoWorld.StoreManager().GetComponentTypesForArchID(gotID)
	assert.Equal(t, 1, len(gotComps))
	assert.Check(t, filter.MatchComponentMetaData(gotComps, twoNum))

	// Archetype indices should be the same across save/load cycles
	assert.Equal(t, wantID, gotID)
}

func TestCanRecoverArchetypeInformationAfterLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)

	oneWorld := ecstestutils.InitWorldWithRedis(t, redisStore)
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[OneAlphaNum](oneWorld))
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[OneBetaNum](oneWorld))
	testutils.AssertNilErrorWithTrace(t, oneWorld.LoadGameState())

	oneWorldCtx := ecs.NewWorldContext(oneWorld)
	_, err := component.Create(oneWorldCtx, OneAlphaNum{})
	testutils.AssertNilErrorWithTrace(t, err)
	_, err = component.Create(oneWorldCtx, OneBetaNum{})
	testutils.AssertNilErrorWithTrace(t, err)
	_, err = component.Create(oneWorldCtx, OneAlphaNum{}, OneBetaNum{})
	testutils.AssertNilErrorWithTrace(t, err)
	oneAlphaNum, err := oneWorld.GetComponentByName(OneAlphaNum{}.Name())
	testutils.AssertNilErrorWithTrace(t, err)
	oneBetaNum, err := oneWorld.GetComponentByName(OneBetaNum{}.Name())
	testutils.AssertNilErrorWithTrace(t, err)
	// At this point 3 archetypes exist:
	// oneAlphaNum
	// oneBetaNum
	// oneAlphaNum, oneBetaNum
	oneJustAlphaArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneAlphaNum))
	testutils.AssertNilErrorWithTrace(t, err)
	oneJustBetaArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneBetaNum))
	testutils.AssertNilErrorWithTrace(t, err)
	oneBothArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneAlphaNum, oneBetaNum))
	testutils.AssertNilErrorWithTrace(t, err)
	// These archetype indices should be preserved between a state save/load

	testutils.AssertNilErrorWithTrace(t, oneWorld.Tick(context.Background()))

	// Create a brand new world, but use the original redis store. We should be able to load
	// the game state from the redis store (including archetype indices).
	twoWorld := ecstestutils.InitWorldWithRedis(t, redisStore)
	// The ordering of registering these components is important. It must match the ordering above.
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[TwoAlphaNum](twoWorld))
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[TwoBetaNum](twoWorld))
	testutils.AssertNilErrorWithTrace(t, twoWorld.LoadGameState())

	// Don't create any entities like above; they should already exist

	// The order that we FETCH archetypes shouldn't matter, so this order is intentionally
	// different from the setup step
	twoBothArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneBetaNum, oneAlphaNum))
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, oneBothArchID, twoBothArchID)
	twoJustAlphaArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneAlphaNum))
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, oneJustAlphaArchID, twoJustAlphaArchID)
	twoJustBetaArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneBetaNum))
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, oneJustBetaArchID, twoJustBetaArchID)

	// Save and load again to make sure the "two" world correctly saves its state even though
	// it never created any entities
	testutils.AssertNilErrorWithTrace(t, twoWorld.Tick(context.Background()))

	threeWorld := ecstestutils.InitWorldWithRedis(t, redisStore)
	// Again, the ordering of registering these components is important. It must match the ordering above
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[ThreeAlphaNum](threeWorld))
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[ThreeBetaNum](threeWorld))
	testutils.AssertNilErrorWithTrace(t, threeWorld.LoadGameState())

	// And again, the loading of archetypes is intentionally different from the above two steps
	threeJustBetaArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneBetaNum))
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, oneJustBetaArchID, threeJustBetaArchID)
	threeBothArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneBetaNum, oneAlphaNum))
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, oneBothArchID, threeBothArchID)
	threeJustAlphaArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneAlphaNum))
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, oneJustAlphaArchID, threeJustAlphaArchID)
}

type OneBetaNum struct {
	Num int
}

func (OneBetaNum) Name() string {
	return "oneBetaNum"
}

type oneAlphaNumComp struct {
	Num int
}

func (oneAlphaNumComp) Name() string {
	return "oneAlphaNum"
}

func TestCanReloadState(t *testing.T) {
	redisStore := miniredis.RunT(t)
	alphaWorld := ecstestutils.InitWorldWithRedis(t, redisStore)
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[oneAlphaNumComp](alphaWorld))

	oneAlphaNum, err := alphaWorld.GetComponentByName(oneAlphaNumComp{}.Name())
	testutils.AssertNilErrorWithTrace(t, err)
	alphaWorld.RegisterSystem(func(wCtx ecs.WorldContext) error {
		q, err := wCtx.NewSearch(ecs.Contains(oneAlphaNum))
		if err != nil {
			return err
		}
		testutils.AssertNilErrorWithTrace(t, q.Each(wCtx, func(id entity.ID) bool {
			err = component.SetComponent[oneAlphaNumComp](wCtx, id, &oneAlphaNumComp{int(id)})
			assert.Check(t, err == nil)
			return true
		}))
		return nil
	})
	testutils.AssertNilErrorWithTrace(t, alphaWorld.LoadGameState())
	_, err = component.CreateMany(ecs.NewWorldContext(alphaWorld), 10, oneAlphaNumComp{})
	testutils.AssertNilErrorWithTrace(t, err)

	// Start a tick with executes the above system which initializes the number components.
	testutils.AssertNilErrorWithTrace(t, alphaWorld.Tick(context.Background()))

	// Make a new world, using the original redis DB that (hopefully) has our data
	betaWorld := ecstestutils.InitWorldWithRedis(t, redisStore)
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[OneBetaNum](betaWorld))
	testutils.AssertNilErrorWithTrace(t, betaWorld.LoadGameState())

	count := 0
	q, err := betaWorld.NewSearch(ecs.Contains(OneBetaNum{}))
	testutils.AssertNilErrorWithTrace(t, err)
	betaWorldCtx := ecs.NewWorldContext(betaWorld)
	testutils.AssertNilErrorWithTrace(t, q.Each(betaWorldCtx, func(id entity.ID) bool {
		count++
		num, err := component.GetComponent[OneBetaNum](betaWorldCtx, id)
		testutils.AssertNilErrorWithTrace(t, err)
		assert.Equal(t, int(id), num.Num)
		return true
	}))
	// Make sure we actually have 10 entities
	assert.Equal(t, 10, count)
}

func TestWorldTickAndHistoryTickMatch(t *testing.T) {
	redisStore := miniredis.RunT(t)
	ctx := context.Background()

	// Ensure that across multiple reloads, getting the transaction receipts for a tick
	// that is still in the tx receipt history window will not return any errors.
	for reload := 0; reload < 5; reload++ {
		world := ecstestutils.InitWorldWithRedis(t, redisStore)
		testutils.AssertNilErrorWithTrace(t, world.LoadGameState())
		relevantTick := world.CurrentTick()
		for i := 0; i < 5; i++ {
			testutils.AssertNilErrorWithTrace(t, world.Tick(ctx))
		}
		// Ignore the actual receipts (they will be empty). Just make sure the tick we're asking
		// for isn't considered too far in the future.
		_, err := world.GetTransactionReceiptsForTick(relevantTick)
		testutils.AssertNilErrorWithTrace(t, err, "error in reload %d", reload)
	}
}

func TestCanFindTransactionsAfterReloadingWorld(t *testing.T) {
	type Msg struct{}
	type Result struct{}
	someTx := ecs.NewMessageType[Msg, Result]("some-msg")
	redisStore := miniredis.RunT(t)
	ctx := context.Background()

	// Ensure that across multiple reloads we can queue up transactions, execute those transactions
	// in a tick, and then find those transactions in the tx receipt history.
	for reload := 0; reload < 5; reload++ {
		world := ecstestutils.InitWorldWithRedis(t, redisStore)
		testutils.AssertNilErrorWithTrace(t, world.RegisterMessages(someTx))
		world.RegisterSystem(func(wCtx ecs.WorldContext) error {
			for _, tx := range someTx.In(wCtx) {
				someTx.SetResult(wCtx, tx.Hash, Result{})
			}
			return nil
		})
		testutils.AssertNilErrorWithTrace(t, world.LoadGameState())

		relevantTick := world.CurrentTick()
		for i := 0; i < 3; i++ {
			_ = someTx.AddToQueue(world, Msg{}, ecstestutils.UniqueSignature(t))
		}

		for i := 0; i < 5; i++ {
			testutils.AssertNilErrorWithTrace(t, world.Tick(ctx))
		}

		receipts, err := world.GetTransactionReceiptsForTick(relevantTick)
		testutils.AssertNilErrorWithTrace(t, err)
		assert.Equal(t, 3, len(receipts))
	}
}
