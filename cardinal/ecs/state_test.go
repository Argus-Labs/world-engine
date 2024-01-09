package ecs_test

import (
	"context"
	"testing"

	"pkg.world.dev/world-engine/assert"

	"github.com/alicebob/miniredis/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

// comps reduces the typing needed to create a slice of IComponentTypes
// []component.ComponentMetadata{a, b, c} becomes:
// comps(a, b, c).
func comps(cs ...component.ComponentMetadata) []component.ComponentMetadata {
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
	// This redisStore will be used to create multiple engines to ensure state is consistent across the engines.
	redisStore := miniredis.RunT(t)

	oneEngine := testutil.InitEngineWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[OneAlphaNum](oneEngine))
	assert.NilError(t, oneEngine.LoadGameState())

	_, err := ecs.Create(ecs.NewEngineContext(oneEngine), OneAlphaNum{})
	assert.NilError(t, err)

	assert.NilError(t, oneEngine.Tick(context.Background()))

	// Too few components registered
	twoEngine := testutil.InitEngineWithRedis(t, redisStore)
	err = twoEngine.LoadGameState()
	assert.ErrorContains(t, err, storage.ErrComponentMismatchWithSavedState.Error())

	// It's ok to register extra components.
	threeEngine := testutil.InitEngineWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[ThreeAlphaNum](threeEngine))
	assert.NilError(t, ecs.RegisterComponent[ThreeBetaNum](threeEngine))
	assert.NilError(t, threeEngine.LoadGameState())

	// Just the right number of components registered
	fourEngine := testutil.InitEngineWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[FoundAlphaNum](fourEngine))
	assert.NilError(t, fourEngine.LoadGameState())
}

func TestArchetypeIDIsConsistentAfterSaveAndLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)
	oneEngine := testutil.InitEngineWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[NumberComponent](oneEngine))
	assert.NilError(t, oneEngine.LoadGameState())

	_, err := ecs.Create(ecs.NewEngineContext(oneEngine), NumberComponent{})
	assert.NilError(t, err)
	oneNum, err := oneEngine.GetComponentMetadataByName(NumberComponent{}.Name())
	assert.NilError(t, err)
	wantID, err := oneEngine.StoreManager().GetArchIDForComponents(comps(oneNum))
	assert.NilError(t, err)
	wantComps := oneEngine.StoreManager().GetComponentTypesForArchID(wantID)
	assert.Equal(t, 1, len(wantComps))
	assert.Check(t, filter.MatchComponentMetaData(wantComps, oneNum))

	assert.NilError(t, oneEngine.Tick(context.Background()))

	// Make a second instance of the engine using the same storage.
	twoEngine := testutil.InitEngineWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[NumberComponent](twoEngine))
	assert.NilError(t, twoEngine.LoadGameState())
	twoNum, err := twoEngine.GetComponentMetadataByName(NumberComponent{}.Name())
	assert.NilError(t, err)
	gotID, err := twoEngine.StoreManager().GetArchIDForComponents(comps(twoNum))
	assert.NilError(t, err)
	gotComps := twoEngine.StoreManager().GetComponentTypesForArchID(gotID)
	assert.Equal(t, 1, len(gotComps))
	assert.Check(t, filter.MatchComponentMetaData(gotComps, twoNum))

	// Archetype indices should be the same across save/load cycles
	assert.Equal(t, wantID, gotID)
}

func TestCanRecoverArchetypeInformationAfterLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)

	oneEngine := testutil.InitEngineWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[OneAlphaNum](oneEngine))
	assert.NilError(t, ecs.RegisterComponent[OneBetaNum](oneEngine))
	assert.NilError(t, oneEngine.LoadGameState())

	oneEngineCtx := ecs.NewEngineContext(oneEngine)
	_, err := ecs.Create(oneEngineCtx, OneAlphaNum{})
	assert.NilError(t, err)
	_, err = ecs.Create(oneEngineCtx, OneBetaNum{})
	assert.NilError(t, err)
	_, err = ecs.Create(oneEngineCtx, OneAlphaNum{}, OneBetaNum{})
	assert.NilError(t, err)
	oneAlphaNum, err := oneEngine.GetComponentMetadataByName(OneAlphaNum{}.Name())
	assert.NilError(t, err)
	oneBetaNum, err := oneEngine.GetComponentMetadataByName(OneBetaNum{}.Name())
	assert.NilError(t, err)
	// At this point 3 archetypes exist:
	// oneAlphaNum
	// oneBetaNum
	// oneAlphaNum, oneBetaNum
	oneJustAlphaArchID, err := oneEngine.StoreManager().GetArchIDForComponents(comps(oneAlphaNum))
	assert.NilError(t, err)
	oneJustBetaArchID, err := oneEngine.StoreManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	oneBothArchID, err := oneEngine.StoreManager().GetArchIDForComponents(comps(oneAlphaNum, oneBetaNum))
	assert.NilError(t, err)
	// These archetype indices should be preserved between a state save/load

	assert.NilError(t, oneEngine.Tick(context.Background()))

	// Create a brand new engine, but use the original redis store. We should be able to load
	// the game state from the redis store (including archetype indices).
	twoEngine := testutil.InitEngineWithRedis(t, redisStore)
	// The ordering of registering these components is important. It must match the ordering above.
	assert.NilError(t, ecs.RegisterComponent[TwoAlphaNum](twoEngine))
	assert.NilError(t, ecs.RegisterComponent[TwoBetaNum](twoEngine))
	assert.NilError(t, twoEngine.LoadGameState())

	// Don't create any entities like above; they should already exist

	// The order that we FETCH archetypes shouldn't matter, so this order is intentionally
	// different from the setup step
	twoBothArchID, err := oneEngine.StoreManager().GetArchIDForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneBothArchID, twoBothArchID)
	twoJustAlphaArchID, err := oneEngine.StoreManager().GetArchIDForComponents(comps(oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustAlphaArchID, twoJustAlphaArchID)
	twoJustBetaArchID, err := oneEngine.StoreManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustBetaArchID, twoJustBetaArchID)

	// Save and load again to make sure the "two" engine correctly saves its state even though
	// it never created any entities
	assert.NilError(t, twoEngine.Tick(context.Background()))

	threeEngine := testutil.InitEngineWithRedis(t, redisStore)
	// Again, the ordering of registering these components is important. It must match the ordering above
	assert.NilError(t, ecs.RegisterComponent[ThreeAlphaNum](threeEngine))
	assert.NilError(t, ecs.RegisterComponent[ThreeBetaNum](threeEngine))
	assert.NilError(t, threeEngine.LoadGameState())

	// And again, the loading of archetypes is intentionally different from the above two steps
	threeJustBetaArchID, err := oneEngine.StoreManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustBetaArchID, threeJustBetaArchID)
	threeBothArchID, err := oneEngine.StoreManager().GetArchIDForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneBothArchID, threeBothArchID)
	threeJustAlphaArchID, err := oneEngine.StoreManager().GetArchIDForComponents(comps(oneAlphaNum))
	assert.NilError(t, err)
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
	alphaEngine := testutil.InitEngineWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[oneAlphaNumComp](alphaEngine))

	oneAlphaNum, err := alphaEngine.GetComponentMetadataByName(oneAlphaNumComp{}.Name())
	assert.NilError(t, err)
	alphaEngine.RegisterSystem(
		func(eCtx ecs.EngineContext) error {
			q, err := eCtx.NewSearch(ecs.Contains(oneAlphaNum))
			if err != nil {
				return err
			}
			assert.NilError(
				t, q.Each(
					eCtx, func(id entity.ID) bool {
						err = ecs.SetComponent[oneAlphaNumComp](eCtx, id, &oneAlphaNumComp{int(id)})
						assert.Check(t, err == nil)
						return true
					},
				),
			)
			return nil
		},
	)
	assert.NilError(t, alphaEngine.LoadGameState())
	_, err = ecs.CreateMany(ecs.NewEngineContext(alphaEngine), 10, oneAlphaNumComp{})
	assert.NilError(t, err)

	// Start a tick with executes the above system which initializes the number components.
	assert.NilError(t, alphaEngine.Tick(context.Background()))

	// Make a new engine, using the original redis DB that (hopefully) has our data
	betaEngine := testutil.InitEngineWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[OneBetaNum](betaEngine))
	assert.NilError(t, betaEngine.LoadGameState())

	count := 0
	q, err := betaEngine.NewSearch(ecs.Contains(OneBetaNum{}))
	assert.NilError(t, err)
	betaEngineCtx := ecs.NewEngineContext(betaEngine)
	assert.NilError(
		t, q.Each(
			betaEngineCtx, func(id entity.ID) bool {
				count++
				num, err := ecs.GetComponent[OneBetaNum](betaEngineCtx, id)
				assert.NilError(t, err)
				assert.Equal(t, int(id), num.Num)
				return true
			},
		),
	)
	// Make sure we actually have 10 entities
	assert.Equal(t, 10, count)
}

func TestEngineTickAndHistoryTickMatch(t *testing.T) {
	redisStore := miniredis.RunT(t)
	ctx := context.Background()

	// Ensure that across multiple reloads, getting the transaction receipts for a tick
	// that is still in the tx receipt history window will not return any errors.
	for reload := 0; reload < 5; reload++ {
		engine := testutil.InitEngineWithRedis(t, redisStore)
		assert.NilError(t, engine.LoadGameState())
		relevantTick := engine.CurrentTick()
		for i := 0; i < 5; i++ {
			assert.NilError(t, engine.Tick(ctx))
		}
		// Ignore the actual receipts (they will be empty). Just make sure the tick we're asking
		// for isn't considered too far in the future.
		_, err := engine.GetTransactionReceiptsForTick(relevantTick)
		assert.NilError(t, err, "error in reload %d", reload)
	}
}

func TestCanFindTransactionsAfterReloadingEngine(t *testing.T) {
	type Msg struct{}
	type Result struct{}
	someTx := ecs.NewMessageType[Msg, Result]("some-msg")
	redisStore := miniredis.RunT(t)
	ctx := context.Background()

	// Ensure that across multiple reloads we can queue up transactions, execute those transactions
	// in a tick, and then find those transactions in the tx receipt history.
	for reload := 0; reload < 5; reload++ {
		engine := testutil.InitEngineWithRedis(t, redisStore)
		assert.NilError(t, engine.RegisterMessages(someTx))
		engine.RegisterSystem(
			func(eCtx ecs.EngineContext) error {
				for _, tx := range someTx.In(eCtx) {
					someTx.SetResult(eCtx, tx.Hash, Result{})
				}
				return nil
			},
		)
		assert.NilError(t, engine.LoadGameState())

		relevantTick := engine.CurrentTick()
		for i := 0; i < 3; i++ {
			_ = someTx.AddToQueue(engine, Msg{}, testutil.UniqueSignature(t))
		}

		for i := 0; i < 5; i++ {
			assert.NilError(t, engine.Tick(ctx))
		}

		receipts, err := engine.GetTransactionReceiptsForTick(relevantTick)
		assert.NilError(t, err)
		assert.Equal(t, 3, len(receipts))
	}
}
