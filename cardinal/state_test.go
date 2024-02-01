package cardinal_test

import (
	"context"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"testing"

	"pkg.world.dev/world-engine/assert"

	"github.com/alicebob/miniredis/v2"

	"pkg.world.dev/world-engine/cardinal/testutils"
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
	// This redisStore will be used to cardinal.Create multiple engines to ensure state is consistent across the engines.
	redisStore := miniredis.RunT(t)

	oneWorld := testutils.NewTestFixture(t, redisStore).World
	assert.NilError(t, cardinal.RegisterComponent[OneAlphaNum](oneWorld))
	assert.NilError(t, oneWorld.LoadGameState())

	_, err := cardinal.Create(cardinal.NewWorldContext(oneWorld), OneAlphaNum{})
	assert.NilError(t, err)

	assert.NilError(t, oneWorld.Tick(context.Background()))

	// Too few components registered
	twoWorld := testutils.NewTestFixture(t, redisStore).World
	err = twoWorld.LoadGameState()
	assert.ErrorContains(t, err, iterators.ErrComponentMismatchWithSavedState.Error())

	// It's ok to register extra components.
	threeWorld := testutils.NewTestFixture(t, redisStore).World
	assert.NilError(t, cardinal.RegisterComponent[ThreeAlphaNum](threeWorld))
	assert.NilError(t, cardinal.RegisterComponent[ThreeBetaNum](threeWorld))
	assert.NilError(t, threeWorld.LoadGameState())

	// Just the right number of components registered
	fourWorld := testutils.NewTestFixture(t, redisStore).World
	assert.NilError(t, cardinal.RegisterComponent[FoundAlphaNum](fourWorld))
	assert.NilError(t, fourWorld.LoadGameState())
}

func TestArchetypeIDIsConsistentAfterSaveAndLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)
	oneWorld := testutils.NewTestFixture(t, redisStore).World
	assert.NilError(t, cardinal.RegisterComponent[NumberComponent](oneWorld))
	assert.NilError(t, oneWorld.LoadGameState())

	_, err := cardinal.Create(cardinal.NewWorldContext(oneWorld), NumberComponent{})
	assert.NilError(t, err)
	oneNum, err := oneWorld.GetComponentByName(NumberComponent{}.Name())
	assert.NilError(t, err)
	wantID, err := oneWorld.GameStateManager().GetArchIDForComponents(comps(oneNum))
	assert.NilError(t, err)
	wantComps := oneWorld.GameStateManager().GetComponentTypesForArchID(wantID)
	assert.Equal(t, 1, len(wantComps))
	assert.Check(t, filter.MatchComponent(component.ConvertComponentMetadatasToComponents(wantComps), oneNum))

	assert.NilError(t, oneWorld.Tick(context.Background()))

	// Make a second instance of the engine using the same storage.
	twoWorld := testutils.NewTestFixture(t, redisStore).World
	assert.NilError(t, cardinal.RegisterComponent[NumberComponent](twoWorld))
	assert.NilError(t, twoWorld.LoadGameState())
	twoNum, err := twoWorld.GetComponentByName(NumberComponent{}.Name())
	assert.NilError(t, err)
	gotID, err := twoWorld.GameStateManager().GetArchIDForComponents(comps(twoNum))
	assert.NilError(t, err)
	gotComps := twoWorld.GameStateManager().GetComponentTypesForArchID(gotID)
	assert.Equal(t, 1, len(gotComps))
	assert.Check(t, filter.MatchComponent(component.ConvertComponentMetadatasToComponents(gotComps), twoNum))

	// Archetype indices should be the same across save/load cycles
	assert.Equal(t, wantID, gotID)
}

func TestCanRecoverArchetypeInformationAfterLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)

	oneWorld := testutils.NewTestFixture(t, redisStore).World
	assert.NilError(t, cardinal.RegisterComponent[OneAlphaNum](oneWorld))
	assert.NilError(t, cardinal.RegisterComponent[OneBetaNum](oneWorld))
	assert.NilError(t, oneWorld.LoadGameState())

	oneEngineCtx := cardinal.NewWorldContext(oneWorld)
	_, err := cardinal.Create(oneEngineCtx, OneAlphaNum{})
	assert.NilError(t, err)
	_, err = cardinal.Create(oneEngineCtx, OneBetaNum{})
	assert.NilError(t, err)
	_, err = cardinal.Create(oneEngineCtx, OneAlphaNum{}, OneBetaNum{})
	assert.NilError(t, err)
	oneAlphaNum, err := oneWorld.GetComponentByName(OneAlphaNum{}.Name())
	assert.NilError(t, err)
	oneBetaNum, err := oneWorld.GetComponentByName(OneBetaNum{}.Name())
	assert.NilError(t, err)
	// At this point 3 archetypes exist:
	// oneAlphaNum
	// oneBetaNum
	// oneAlphaNum, oneBetaNum
	oneJustAlphaArchID, err := oneWorld.GameStateManager().GetArchIDForComponents(comps(oneAlphaNum))
	assert.NilError(t, err)
	oneJustBetaArchID, err := oneWorld.GameStateManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	oneBothArchID, err := oneWorld.GameStateManager().GetArchIDForComponents(comps(oneAlphaNum, oneBetaNum))
	assert.NilError(t, err)
	// These archetype indices should be preserved between a state save/load

	assert.NilError(t, oneWorld.Tick(context.Background()))

	// cardinal.Create a brand new engine, but use the original redis store. We should be able to load
	// the game state from the redis store (including archetype indices).
	twoWorld := testutils.NewTestFixture(t, redisStore).World
	// The ordering of registering these components is important. It must match the ordering above.
	assert.NilError(t, cardinal.RegisterComponent[TwoAlphaNum](twoWorld))
	assert.NilError(t, cardinal.RegisterComponent[TwoBetaNum](twoWorld))
	assert.NilError(t, twoWorld.LoadGameState())

	// Don't cardinal.Create any entities like above; they should already exist

	// The order that we FETCH archetypes shouldn't matter, so this order is intentionally
	// different from the setup step
	twoBothArchID, err := oneWorld.GameStateManager().GetArchIDForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneBothArchID, twoBothArchID)
	twoJustAlphaArchID, err := oneWorld.GameStateManager().GetArchIDForComponents(comps(oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustAlphaArchID, twoJustAlphaArchID)
	twoJustBetaArchID, err := oneWorld.GameStateManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustBetaArchID, twoJustBetaArchID)

	// Save and load again to make sure the "two" engine correctly saves its state even though
	// it never cardinal.Created any entities
	assert.NilError(t, twoWorld.Tick(context.Background()))

	threeWorld := testutils.NewTestFixture(t, redisStore).World
	// Again, the ordering of registering these components is important. It must match the ordering above
	assert.NilError(t, cardinal.RegisterComponent[ThreeAlphaNum](threeWorld))
	assert.NilError(t, cardinal.RegisterComponent[ThreeBetaNum](threeWorld))
	assert.NilError(t, threeWorld.LoadGameState())

	// And again, the loading of archetypes is intentionally different from the above two steps
	threeJustBetaArchID, err := oneWorld.GameStateManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustBetaArchID, threeJustBetaArchID)
	threeBothArchID, err := oneWorld.GameStateManager().GetArchIDForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneBothArchID, threeBothArchID)
	threeJustAlphaArchID, err := oneWorld.GameStateManager().GetArchIDForComponents(comps(oneAlphaNum))
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
	alphaWorld := testutils.NewTestFixture(t, redisStore).World
	assert.NilError(t, cardinal.RegisterComponent[oneAlphaNumComp](alphaWorld))

	oneAlphaNum, err := alphaWorld.GetComponentByName(oneAlphaNumComp{}.Name())
	assert.NilError(t, err)
	err = cardinal.RegisterSystems(
		alphaWorld,
		func(eCtx engine.Context) error {
			q := cardinal.NewSearch(eCtx, filter.Contains(oneAlphaNum))
			assert.NilError(
				t, q.Each(
					func(id entity.ID) bool {
						err = cardinal.SetComponent[oneAlphaNumComp](eCtx, id, &oneAlphaNumComp{int(id)})
						assert.Check(t, err == nil)
						return true
					},
				),
			)
			return nil
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, alphaWorld.LoadGameState())
	_, err = cardinal.CreateMany(cardinal.NewWorldContext(alphaWorld), 10, oneAlphaNumComp{})
	assert.NilError(t, err)

	// Start a tick with executes the above system which initializes the number components.
	assert.NilError(t, alphaWorld.Tick(context.Background()))

	// Make a new engine, using the original redis DB that (hopefully) has our data
	betaWorld := testutils.NewTestFixture(t, redisStore).World
	assert.NilError(t, cardinal.RegisterComponent[OneBetaNum](betaWorld))
	assert.NilError(t, betaWorld.LoadGameState())

	count := 0
	q := cardinal.NewSearch(cardinal.NewWorldContext(betaWorld), filter.Contains(OneBetaNum{}))
	betaEngineCtx := cardinal.NewWorldContext(betaWorld)
	assert.NilError(
		t, q.Each(
			func(id entity.ID) bool {
				count++
				num, err := cardinal.GetComponent[OneBetaNum](betaEngineCtx, id)
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
		world := testutils.NewTestFixture(t, redisStore).World
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

func TestCanFindTransactionsAfterReloadingEngine(t *testing.T) {
	type Msg struct{}
	type Result struct{}
	someTx := cardinal.NewMessageType[Msg, Result]("some-msg")
	redisStore := miniredis.RunT(t)
	ctx := context.Background()

	// Ensure that across multiple reloads we can queue up transactions, execute those transactions
	// in a tick, and then find those transactions in the tx receipt history.
	for reload := 0; reload < 5; reload++ {
		world := testutils.NewTestFixture(t, redisStore).World
		assert.NilError(t, cardinal.RegisterMessages(world, someTx))
		err := cardinal.RegisterSystems(
			world,
			func(eCtx engine.Context) error {
				for _, tx := range someTx.In(eCtx) {
					someTx.SetResult(eCtx, tx.Hash, Result{})
				}
				return nil
			},
		)
		assert.NilError(t, err)
		assert.NilError(t, world.LoadGameState())

		relevantTick := world.CurrentTick()
		for i := 0; i < 3; i++ {
			_ = someTx.AddToQueue(world, Msg{}, testutils.UniqueSignature())
		}

		for i := 0; i < 5; i++ {
			assert.NilError(t, world.Tick(ctx))
		}

		receipts, err := world.GetTransactionReceiptsForTick(relevantTick)
		assert.NilError(t, err)
		assert.Equal(t, 3, len(receipts))
	}
}
