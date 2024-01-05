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
	// This redisStore will be used to create multiple worlds to ensure state is consistent across the worlds.
	redisStore := miniredis.RunT(t)

	oneWorld := testutil.InitWorldWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[OneAlphaNum](oneWorld))
	assert.NilError(t, oneWorld.LoadGameState())

	_, err := ecs.Create(ecs.NewWorldContext(oneWorld), OneAlphaNum{})
	assert.NilError(t, err)

	assert.NilError(t, oneWorld.Tick(context.Background()))

	// Too few components registered
	twoWorld := testutil.InitWorldWithRedis(t, redisStore)
	err = twoWorld.LoadGameState()
	assert.ErrorContains(t, err, storage.ErrComponentMismatchWithSavedState.Error())

	// It's ok to register extra components.
	threeWorld := testutil.InitWorldWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[ThreeAlphaNum](threeWorld))
	assert.NilError(t, ecs.RegisterComponent[ThreeBetaNum](threeWorld))
	assert.NilError(t, threeWorld.LoadGameState())

	// Just the right number of components registered
	fourWorld := testutil.InitWorldWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[FoundAlphaNum](fourWorld))
	assert.NilError(t, fourWorld.LoadGameState())
}

func TestArchetypeIDIsConsistentAfterSaveAndLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)
	oneWorld := testutil.InitWorldWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[NumberComponent](oneWorld))
	assert.NilError(t, oneWorld.LoadGameState())

	_, err := ecs.Create(ecs.NewWorldContext(oneWorld), NumberComponent{})
	assert.NilError(t, err)
	oneNum, err := oneWorld.GetComponentByName(NumberComponent{}.Name())
	assert.NilError(t, err)
	wantID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneNum))
	assert.NilError(t, err)
	wantComps := oneWorld.StoreManager().GetComponentTypesForArchID(wantID)
	assert.Equal(t, 1, len(wantComps))
	assert.Check(t, filter.MatchComponentMetaData(wantComps, oneNum))

	assert.NilError(t, oneWorld.Tick(context.Background()))

	// Make a second instance of the world using the same storage.
	twoWorld := testutil.InitWorldWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[NumberComponent](twoWorld))
	assert.NilError(t, twoWorld.LoadGameState())
	twoNum, err := twoWorld.GetComponentByName(NumberComponent{}.Name())
	assert.NilError(t, err)
	gotID, err := twoWorld.StoreManager().GetArchIDForComponents(comps(twoNum))
	assert.NilError(t, err)
	gotComps := twoWorld.StoreManager().GetComponentTypesForArchID(gotID)
	assert.Equal(t, 1, len(gotComps))
	assert.Check(t, filter.MatchComponentMetaData(gotComps, twoNum))

	// Archetype indices should be the same across save/load cycles
	assert.Equal(t, wantID, gotID)
}

func TestCanRecoverArchetypeInformationAfterLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)

	oneWorld := testutil.InitWorldWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[OneAlphaNum](oneWorld))
	assert.NilError(t, ecs.RegisterComponent[OneBetaNum](oneWorld))
	assert.NilError(t, oneWorld.LoadGameState())

	oneWorldCtx := ecs.NewWorldContext(oneWorld)
	_, err := ecs.Create(oneWorldCtx, OneAlphaNum{})
	assert.NilError(t, err)
	_, err = ecs.Create(oneWorldCtx, OneBetaNum{})
	assert.NilError(t, err)
	_, err = ecs.Create(oneWorldCtx, OneAlphaNum{}, OneBetaNum{})
	assert.NilError(t, err)
	oneAlphaNum, err := oneWorld.GetComponentByName(OneAlphaNum{}.Name())
	assert.NilError(t, err)
	oneBetaNum, err := oneWorld.GetComponentByName(OneBetaNum{}.Name())
	assert.NilError(t, err)
	// At this point 3 archetypes exist:
	// oneAlphaNum
	// oneBetaNum
	// oneAlphaNum, oneBetaNum
	oneJustAlphaArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneAlphaNum))
	assert.NilError(t, err)
	oneJustBetaArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	oneBothArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneAlphaNum, oneBetaNum))
	assert.NilError(t, err)
	// These archetype indices should be preserved between a state save/load

	assert.NilError(t, oneWorld.Tick(context.Background()))

	// Create a brand new world, but use the original redis store. We should be able to load
	// the game state from the redis store (including archetype indices).
	twoWorld := testutil.InitWorldWithRedis(t, redisStore)
	// The ordering of registering these components is important. It must match the ordering above.
	assert.NilError(t, ecs.RegisterComponent[TwoAlphaNum](twoWorld))
	assert.NilError(t, ecs.RegisterComponent[TwoBetaNum](twoWorld))
	assert.NilError(t, twoWorld.LoadGameState())

	// Don't create any entities like above; they should already exist

	// The order that we FETCH archetypes shouldn't matter, so this order is intentionally
	// different from the setup step
	twoBothArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneBothArchID, twoBothArchID)
	twoJustAlphaArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustAlphaArchID, twoJustAlphaArchID)
	twoJustBetaArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustBetaArchID, twoJustBetaArchID)

	// Save and load again to make sure the "two" world correctly saves its state even though
	// it never created any entities
	assert.NilError(t, twoWorld.Tick(context.Background()))

	threeWorld := testutil.InitWorldWithRedis(t, redisStore)
	// Again, the ordering of registering these components is important. It must match the ordering above
	assert.NilError(t, ecs.RegisterComponent[ThreeAlphaNum](threeWorld))
	assert.NilError(t, ecs.RegisterComponent[ThreeBetaNum](threeWorld))
	assert.NilError(t, threeWorld.LoadGameState())

	// And again, the loading of archetypes is intentionally different from the above two steps
	threeJustBetaArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustBetaArchID, threeJustBetaArchID)
	threeBothArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneBothArchID, threeBothArchID)
	threeJustAlphaArchID, err := oneWorld.StoreManager().GetArchIDForComponents(comps(oneAlphaNum))
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
	alphaWorld := testutil.InitWorldWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[oneAlphaNumComp](alphaWorld))

	oneAlphaNum, err := alphaWorld.GetComponentByName(oneAlphaNumComp{}.Name())
	assert.NilError(t, err)
	alphaWorld.RegisterSystem(
		func(wCtx ecs.WorldContext) error {
			q, err := wCtx.NewSearch(ecs.Contains(oneAlphaNum))
			if err != nil {
				return err
			}
			assert.NilError(
				t, q.Each(
					wCtx, func(id entity.ID) bool {
						err = ecs.SetComponent[oneAlphaNumComp](wCtx, id, &oneAlphaNumComp{int(id)})
						assert.Check(t, err == nil)
						return true
					},
				).Commit(),
			)
			return nil
		},
	)
	assert.NilError(t, alphaWorld.LoadGameState())
	_, err = ecs.CreateMany(ecs.NewWorldContext(alphaWorld), 10, oneAlphaNumComp{})
	assert.NilError(t, err)

	// Start a tick with executes the above system which initializes the number components.
	assert.NilError(t, alphaWorld.Tick(context.Background()))

	// Make a new world, using the original redis DB that (hopefully) has our data
	betaWorld := testutil.InitWorldWithRedis(t, redisStore)
	assert.NilError(t, ecs.RegisterComponent[OneBetaNum](betaWorld))
	assert.NilError(t, betaWorld.LoadGameState())

	count := 0
	q, err := betaWorld.NewSearch(ecs.Contains(OneBetaNum{}))
	assert.NilError(t, err)
	betaWorldCtx := ecs.NewWorldContext(betaWorld)
	assert.NilError(
		t, q.Each(
			betaWorldCtx, func(id entity.ID) bool {
				count++
				num, err := ecs.GetComponent[OneBetaNum](betaWorldCtx, id)
				assert.NilError(t, err)
				assert.Equal(t, int(id), num.Num)
				return true
			},
		).Commit(),
	)
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
	someTx := ecs.NewMessageType[Msg, Result]("some-msg")
	redisStore := miniredis.RunT(t)
	ctx := context.Background()

	// Ensure that across multiple reloads we can queue up transactions, execute those transactions
	// in a tick, and then find those transactions in the tx receipt history.
	for reload := 0; reload < 5; reload++ {
		world := testutil.InitWorldWithRedis(t, redisStore)
		assert.NilError(t, world.RegisterMessages(someTx))
		world.RegisterSystem(
			func(wCtx ecs.WorldContext) error {
				for _, tx := range someTx.In(wCtx) {
					someTx.SetResult(wCtx, tx.Hash, Result{})
				}
				return nil
			},
		)
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
