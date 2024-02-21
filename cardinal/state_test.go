package cardinal_test

import (
	"context"
	"testing"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"

	"pkg.world.dev/world-engine/assert"

	"github.com/alicebob/miniredis/v2"

	"pkg.world.dev/world-engine/cardinal/testutils"
)

// comps reduces the typing needed to create a slice of IComponentTypes
// []component.ComponentMetadata{a, b, c} becomes:
// comps(a, b, c).
func comps(cs ...types.ComponentMetadata) []types.ComponentMetadata {
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

	tf1 := testutils.NewTestFixture(t, redisStore)
	world1 := tf1.World
	assert.NilError(t, cardinal.RegisterComponent[OneAlphaNum](world1))
	tf1.StartWorld()

	_, err := cardinal.Create(cardinal.NewWorldContext(world1), OneAlphaNum{})
	assert.NilError(t, err)
	tf1.DoTick()

	// Too few components registered
	tf2 := testutils.NewTestFixture(t, redisStore)
	err = tf2.World.StartGame() // We start this manually instead of tf2.StartWorld() because StartWorld panics on err
	assert.ErrorContains(t, err, iterators.ErrComponentMismatchWithSavedState.Error())

	// It's ok to register extra components.
	tf3 := testutils.NewTestFixture(t, redisStore)
	world3 := tf3.World
	assert.NilError(t, cardinal.RegisterComponent[ThreeAlphaNum](world3))
	assert.NilError(t, cardinal.RegisterComponent[ThreeBetaNum](world3))
	tf3.StartWorld()

	// Just the right number of components registered
	tf4 := testutils.NewTestFixture(t, redisStore)
	world4 := tf4.World
	assert.NilError(t, cardinal.RegisterComponent[FoundAlphaNum](world4))
	tf4.StartWorld()
}

func TestArchetypeIDIsConsistentAfterSaveAndLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)
	tf1 := testutils.NewTestFixture(t, redisStore)
	world1 := tf1.World
	assert.NilError(t, cardinal.RegisterComponent[NumberComponent](world1))
	tf1.StartWorld()

	_, err := cardinal.Create(cardinal.NewWorldContext(world1), NumberComponent{})
	assert.NilError(t, err)
	oneNum, err := world1.GetComponentByName(NumberComponent{}.Name())
	assert.NilError(t, err)
	wantID, err := world1.GameStateManager().GetArchIDForComponents(comps(oneNum))
	assert.NilError(t, err)
	wantComps := world1.GameStateManager().GetComponentTypesForArchID(wantID)
	assert.Equal(t, 1, len(wantComps))
	assert.Check(t, filter.MatchComponent(types.ConvertComponentMetadatasToComponents(wantComps), oneNum))

	assert.NilError(t, world1.Tick(context.Background()))

	// Make a second instance of the engine using the same storage.
	tf2 := testutils.NewTestFixture(t, redisStore)
	world2 := tf2.World
	assert.NilError(t, cardinal.RegisterComponent[NumberComponent](world2))
	tf2.StartWorld()
	twoNum, err := world2.GetComponentByName(NumberComponent{}.Name())
	assert.NilError(t, err)
	gotID, err := world2.GameStateManager().GetArchIDForComponents(comps(twoNum))
	assert.NilError(t, err)
	gotComps := world2.GameStateManager().GetComponentTypesForArchID(gotID)
	assert.Equal(t, 1, len(gotComps))
	assert.Check(t, filter.MatchComponent(types.ConvertComponentMetadatasToComponents(gotComps), twoNum))

	// Archetype indices should be the same across save/load cycles
	assert.Equal(t, wantID, gotID)
}

func TestCanRecoverArchetypeInformationAfterLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)

	tf1 := testutils.NewTestFixture(t, redisStore)
	world1 := tf1.World
	assert.NilError(t, cardinal.RegisterComponent[OneAlphaNum](world1))
	assert.NilError(t, cardinal.RegisterComponent[OneBetaNum](world1))
	tf1.StartWorld()

	world1Ctx := cardinal.NewWorldContext(world1)
	_, err := cardinal.Create(world1Ctx, OneAlphaNum{})
	assert.NilError(t, err)
	_, err = cardinal.Create(world1Ctx, OneBetaNum{})
	assert.NilError(t, err)
	_, err = cardinal.Create(world1Ctx, OneAlphaNum{}, OneBetaNum{})
	assert.NilError(t, err)
	oneAlphaNum, err := world1.GetComponentByName(OneAlphaNum{}.Name())
	assert.NilError(t, err)
	oneBetaNum, err := world1.GetComponentByName(OneBetaNum{}.Name())
	assert.NilError(t, err)
	// At this point 3 archetypes exist:
	// world1AlphaNum
	// world1BetaNum
	// world1AlphaNum, oneBetaNum
	world1JustAlphaArchID, err := world1.GameStateManager().GetArchIDForComponents(comps(oneAlphaNum))
	assert.NilError(t, err)
	oneJustBetaArchID, err := world1.GameStateManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	oneBothArchID, err := world1.GameStateManager().GetArchIDForComponents(comps(oneAlphaNum, oneBetaNum))
	assert.NilError(t, err)
	// These archetype indices should be preserved between a state save/load

	assert.NilError(t, world1.Tick(context.Background()))

	// Create a brand new engine, but use the original redis store. We should be able to load
	// the game state from the redis store (including archetype indices).
	tf2 := testutils.NewTestFixture(t, redisStore)
	world2 := tf2.World
	// The ordering of registering these components is important. It must match the ordering above.
	assert.NilError(t, cardinal.RegisterComponent[TwoAlphaNum](world2))
	assert.NilError(t, cardinal.RegisterComponent[TwoBetaNum](world2))
	tf2.StartWorld()

	// Don't create any entities like above; they should already exist

	// The order that we FETCH archetypes shouldn't matter, so this order is intentionally
	// different from the setup step
	world2BothArchID, err := world1.GameStateManager().GetArchIDForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneBothArchID, world2BothArchID)
	twoJustAlphaArchID, err := world1.GameStateManager().GetArchIDForComponents(comps(oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, world1JustAlphaArchID, twoJustAlphaArchID)
	twoJustBetaArchID, err := world1.GameStateManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustBetaArchID, twoJustBetaArchID)

	// Save and load again to make sure the "two" engine correctly saves its state even though
	// it never cardinal.Created any entities
	assert.NilError(t, world2.Tick(context.Background()))

	tf3 := testutils.NewTestFixture(t, redisStore)
	world3 := tf3.World
	// Again, the ordering of registering these components is important. It must match the ordering above
	assert.NilError(t, cardinal.RegisterComponent[ThreeAlphaNum](world3))
	assert.NilError(t, cardinal.RegisterComponent[ThreeBetaNum](world3))
	tf3.StartWorld()

	// And again, the loading of archetypes is intentionally different from the above two steps
	world3JustBetaArchID, err := world1.GameStateManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustBetaArchID, world3JustBetaArchID)
	world3BothArchID, err := world1.GameStateManager().GetArchIDForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneBothArchID, world3BothArchID)
	world3JustAlphaArchID, err := world1.GameStateManager().GetArchIDForComponents(comps(oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, world1JustAlphaArchID, world3JustAlphaArchID)
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
	tf1 := testutils.NewTestFixture(t, redisStore)
	world1 := tf1.World
	assert.NilError(t, cardinal.RegisterComponent[oneAlphaNumComp](world1))

	world1AlphaNum, err := world1.GetComponentByName(oneAlphaNumComp{}.Name())
	assert.NilError(t, err)
	err = cardinal.RegisterSystems(
		world1,
		func(wCtx engine.Context) error {
			q := cardinal.NewSearch(wCtx, filter.Contains(world1AlphaNum))
			assert.NilError(
				t, q.Each(
					func(id types.EntityID) bool {
						err = cardinal.SetComponent[oneAlphaNumComp](wCtx, id, &oneAlphaNumComp{int(id)})
						assert.Check(t, err == nil)
						return true
					},
				),
			)
			return nil
		},
	)
	assert.NilError(t, err)
	tf1.StartWorld()
	_, err = cardinal.CreateMany(cardinal.NewWorldContext(world1), 10, oneAlphaNumComp{})
	assert.NilError(t, err)

	// Start a tick with executes the above system which initializes the number components.
	assert.NilError(t, world1.Tick(context.Background()))

	// Make a new engine, using the original redis DB that (hopefully) has our data
	tf2 := testutils.NewTestFixture(t, redisStore)
	world2 := tf2.World
	assert.NilError(t, cardinal.RegisterComponent[OneBetaNum](world2))
	tf2.StartWorld()

	count := 0
	q := cardinal.NewSearch(cardinal.NewWorldContext(world2), filter.Contains(OneBetaNum{}))
	betaWorldCtx := cardinal.NewWorldContext(world2)
	assert.NilError(
		t, q.Each(
			func(id types.EntityID) bool {
				count++
				num, err := cardinal.GetComponent[OneBetaNum](betaWorldCtx, id)
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
		tf := testutils.NewTestFixture(t, redisStore)
		world := tf.World
		tf.StartWorld()
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
	someTx := message.NewMessageType[Msg, Result]("some-msg")
	redisStore := miniredis.RunT(t)
	ctx := context.Background()

	// Ensure that across multiple reloads we can queue up transactions, execute those transactions
	// in a tick, and then find those transactions in the tx receipt history.
	for reload := 0; reload < 5; reload++ {
		tf := testutils.NewTestFixture(t, redisStore)
		world := tf.World
		assert.NilError(t, cardinal.RegisterMessages(world, someTx))
		err := cardinal.RegisterSystems(
			world,
			func(wCtx engine.Context) error {
				for _, tx := range someTx.In(wCtx) {
					someTx.SetResult(wCtx, tx.Hash, Result{})
				}
				return nil
			},
		)
		assert.NilError(t, err)
		tf.StartWorld()

		relevantTick := world.CurrentTick()
		for i := 0; i < 3; i++ {
			_ = tf.AddTransaction(someTx.ID(), Msg{}, testutils.UniqueSignature())
		}

		for i := 0; i < 5; i++ {
			assert.NilError(t, world.Tick(ctx))
		}

		receipts, err := world.GetTransactionReceiptsForTick(relevantTick)
		assert.NilError(t, err)
		assert.Equal(t, 3, len(receipts))
	}
}

type FooComponent struct {
	Data string
}

func (FooComponent) Name() string {
	return "foo"
}

func TestSearchEarlyTermination(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[FooComponent](world))
	tf.StartWorld()

	total := 10
	count := 0
	stop := 5
	wCtx := cardinal.NewWorldContext(world)
	_, err := cardinal.CreateMany(wCtx, total, FooComponent{})
	assert.NilError(t, err)
	q := cardinal.NewSearch(wCtx, filter.Exact(FooComponent{}))
	assert.NilError(
		t, q.Each(
			func(id types.EntityID) bool {
				count++
				return count != stop
			},
		),
	)
	assert.Equal(t, count, stop)

	count = 0
	q = cardinal.NewSearch(wCtx, filter.Exact(FooComponent{}))
	assert.NilError(
		t, q.Each(
			func(id types.EntityID) bool {
				count++
				return true
			},
		),
	)
	assert.Equal(t, count, total)
}
