package cardinal_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
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

type FooComponent struct {
	Data string
}

func (FooComponent) Name() string {
	return "foo"
}

func TestErrorWhenSavedArchetypesDoNotMatchComponentTypes(t *testing.T) {
	// This redisStore will be used to cardinal.Create multiple engines to ensure state is consistent across the engines.
	tf1 := testutils.NewTestFixture(t, nil)
	world1 := tf1.World
	assert.NilError(t, cardinal.RegisterComponent[OneAlphaNum](world1))
	tf1.StartWorld()

	_, err := cardinal.Create(cardinal.NewWorldContext(world1), OneAlphaNum{})
	assert.NilError(t, err)
	tf1.DoTick()

	// Too few components registered
	tf2 := testutils.NewTestFixture(t, tf1.Redis)
	err = tf2.World.StartGame() // We start this manually instead of tf2.StartWorld() because StartWorld panics on err
	assert.ErrorContains(t, err, iterators.ErrComponentMismatchWithSavedState.Error())

	// It's ok to register extra components.
	tf3 := testutils.NewTestFixture(t, tf1.Redis)
	world3 := tf3.World
	assert.NilError(t, cardinal.RegisterComponent[ThreeAlphaNum](world3))
	assert.NilError(t, cardinal.RegisterComponent[ThreeBetaNum](world3))
	tf3.StartWorld()

	// Just the right number of components registered
	tf4 := testutils.NewTestFixture(t, tf1.Redis)
	world4 := tf4.World
	assert.NilError(t, cardinal.RegisterComponent[FoundAlphaNum](world4))
	tf4.StartWorld()
}

func TestArchetypeIDIsConsistentAfterSaveAndLoad(t *testing.T) {
	tf1 := testutils.NewTestFixture(t, nil)
	world1 := tf1.World
	assert.NilError(t, cardinal.RegisterComponent[NumberComponent](world1))
	tf1.StartWorld()

	_, err := cardinal.Create(cardinal.NewWorldContext(world1), NumberComponent{})
	assert.NilError(t, err)
	oneNum, err := world1.GetComponentByName(NumberComponent{}.Name())
	assert.NilError(t, err)
	wantID, err := world1.GameStateManager().GetArchIDForComponents(comps(oneNum))
	assert.NilError(t, err)
	wantComps, err := world1.GameStateManager().GetComponentTypesForArchID(wantID)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(wantComps))
	matchComponent := filter.CreateComponentMatcher(types.ConvertComponentMetadatasToComponents(wantComps))
	assert.Check(t, matchComponent(oneNum))

	tf1.DoTick()

	// Make a second instance of the engine using the same storage.
	tf2 := testutils.NewTestFixture(t, tf1.Redis)
	world2 := tf2.World
	assert.NilError(t, cardinal.RegisterComponent[NumberComponent](world2))
	tf2.StartWorld()
	twoNum, err := world2.GetComponentByName(NumberComponent{}.Name())
	assert.NilError(t, err)
	gotID, err := world2.GameStateManager().GetArchIDForComponents(comps(twoNum))
	assert.NilError(t, err)
	gotComps, err := world2.GameStateManager().GetComponentTypesForArchID(gotID)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(gotComps))
	matchComponent = filter.CreateComponentMatcher(types.ConvertComponentMetadatasToComponents(gotComps))
	assert.Check(t, matchComponent(twoNum))

	// Archetype indices should be the same across save/load cycles
	assert.Equal(t, wantID, gotID)
}

func TestCanRecoverArchetypeInformationAfterLoad(t *testing.T) {
	mr := miniredis.RunT(t)
	tf1 := testutils.NewTestFixture(t, mr)
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

	tf1.DoTick()

	// Create a brand new engine, but use the original redis store. We should be able to load
	// the game state from the redis store (including archetype indices).
	tf2 := testutils.NewTestFixture(t, mr)
	world2 := tf2.World
	// The ordering of registering these components is important. It must match the ordering above.
	assert.NilError(t, cardinal.RegisterComponent[TwoAlphaNum](world2))
	assert.NilError(t, cardinal.RegisterComponent[TwoBetaNum](world2))
	tf2.StartWorld()

	// Don't create any entities like above; they should already exist

	// The order that we FETCH archetypes shouldn't matter, so this order is intentionally
	// different from the setup step
	world2BothArchID, err := world2.GameStateManager().GetArchIDForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneBothArchID, world2BothArchID)
	twoJustAlphaArchID, err := world2.GameStateManager().GetArchIDForComponents(comps(oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, world1JustAlphaArchID, twoJustAlphaArchID)
	twoJustBetaArchID, err := world2.GameStateManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustBetaArchID, twoJustBetaArchID)

	// Save and load again to make sure the "two" engine correctly saves its state even though
	// it never cardinal.Created any entities
	tf1.DoTick()

	tf3 := testutils.NewTestFixture(t, mr)
	world3 := tf3.World
	// Again, the ordering of registering these components is important. It must match the ordering above
	assert.NilError(t, cardinal.RegisterComponent[ThreeAlphaNum](world3))
	assert.NilError(t, cardinal.RegisterComponent[ThreeBetaNum](world3))
	tf3.StartWorld()

	// And again, the loading of archetypes is intentionally different from the above two steps
	world3JustBetaArchID, err := world3.GameStateManager().GetArchIDForComponents(comps(oneBetaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneJustBetaArchID, world3JustBetaArchID)
	world3BothArchID, err := world3.GameStateManager().GetArchIDForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.NilError(t, err)
	assert.Equal(t, oneBothArchID, world3BothArchID)
	world3JustAlphaArchID, err := world3.GameStateManager().GetArchIDForComponents(comps(oneAlphaNum))
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
	tf1 := testutils.NewTestFixture(t, nil)
	world1 := tf1.World
	assert.NilError(t, cardinal.RegisterComponent[oneAlphaNumComp](world1))

	err := cardinal.RegisterSystems(
		world1,
		func(wCtx cardinal.WorldContext) error {
			q := cardinal.NewSearch().Entity(filter.Contains(filter.Component[oneAlphaNumComp]()))
			assert.NilError(
				t, q.Each(wCtx,
					func(id types.EntityID) bool {
						err := cardinal.SetComponent[oneAlphaNumComp](wCtx, id, &oneAlphaNumComp{int(id)})
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
	tf1.DoTick()

	// Make a new engine, using the original redis DB that (hopefully) has our data
	tf2 := testutils.NewTestFixture(t, tf1.Redis)
	world2 := tf2.World
	assert.NilError(t, cardinal.RegisterComponent[OneBetaNum](world2))
	tf2.StartWorld()

	count := 0
	q := cardinal.NewSearch().Entity(filter.Contains(filter.Component[OneBetaNum]()))
	betaWorldCtx := cardinal.NewWorldContext(world2)
	assert.NilError(
		t, q.Each(cardinal.NewWorldContext(world2),
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
	// Ensure that across multiple reloads, getting the transaction receipts for a tick
	// that is still in the tx receipt history window will not return any errors.
	for reload := 0; reload < 5; reload++ {
		tf := testutils.NewTestFixture(t, nil)
		world := tf.World
		tf.StartWorld()
		relevantTick := world.CurrentTick()
		for i := 0; i < 5; i++ {
			tf.DoTick()
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

	// Ensure that across multiple reloads we can queue up transactions, execute those transactions
	// in a tick, and then find those transactions in the tx receipt history.
	for reload := 0; reload < 5; reload++ {
		tf := testutils.NewTestFixture(t, nil)
		world := tf.World
		msgName := "some-msg"
		assert.NilError(t, cardinal.RegisterMessage[Msg, Result](world, msgName))
		err := cardinal.RegisterSystems(
			world,
			func(wCtx cardinal.WorldContext) error {
				someTx, err := cardinal.GetMessage[Msg, Result](wCtx)
				return cardinal.EachMessage[Msg, Result](wCtx, func(tx cardinal.TxData[Msg]) (Result, error) {
					someTx.SetResult(wCtx, tx.Hash, Result{})
					return Result{}, err
				})
			},
		)
		assert.NilError(t, err)
		tf.StartWorld()

		relevantTick := world.CurrentTick()
		someTx, ok := world.GetMessageByFullName("game." + msgName)
		assert.Assert(t, ok)
		for i := 0; i < 3; i++ {
			_ = tf.AddTransaction(someTx.ID(), Msg{}, testutils.UniqueSignature())
		}

		for i := 0; i < 5; i++ {
			tf.DoTick()
		}

		receipts, err := world.GetTransactionReceiptsForTick(relevantTick)
		assert.NilError(t, err)
		assert.Equal(t, 3, len(receipts))
	}
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
	q := cardinal.NewSearch().Entity(filter.Exact(filter.Component[FooComponent]()))
	assert.NilError(
		t, q.Each(wCtx,
			func(types.EntityID) bool {
				count++
				return count != stop
			},
		),
	)
	assert.Equal(t, count, stop)

	count = 0
	q = cardinal.NewSearch().Entity(filter.Exact(filter.Component[FooComponent]()))
	assert.NilError(
		t, q.Each(wCtx,
			func(types.EntityID) bool {
				count++
				return true
			},
		),
	)
	assert.Equal(t, count, total)
}
