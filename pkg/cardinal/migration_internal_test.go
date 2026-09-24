package cardinal

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	early "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/plugin/earlyaccess/component"
	full "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/plugin/fullrelease/component"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// The stored value and what the plugin's two conversions make of it. Written out rather than
// derived, so a test computing its expectation from the code under test cannot agree with a bug.
const (
	providedStored uint32 = 4
	providedWantC  uint32 = 15 // (4 + 1) * 3
)

// providedV1ToV2 is the plugin's first conversion. It is declared exactly as a plugin author would
// declare it, with In and Out naming the shapes rather than anything about the plugin.
type providedV1ToV2 struct {
	Old In[full.ProvidedV1]
	New Out[full.ProvidedV2]

	calls int
}

func (m *providedV1ToV2) Migrate() {
	m.New.Set(full.ProvidedV2{B: m.Old.Get().A + 1})
	m.calls++
}

// providedV2ToCurrent is the second conversion. Its input is the previous one's output, which is a
// shape no save in this test holds.
type providedV2ToCurrent struct {
	Old In[full.ProvidedV2]
	New Out[full.Provided]

	calls int
}

func (m *providedV2ToCurrent) Migrate() {
	m.New.Set(full.Provided{C: m.Old.Get().B * 3})
	m.calls++
}

// providedPlugin is a plugin that declares migrations for the component it owns, which is the only
// thing this fixture exists to show.
type providedPlugin struct {
	first  *providedV1ToV2
	second *providedV2ToCurrent
}

func (p *providedPlugin) Register(w *World) {
	// Registered second step first, to show the order a plugin registers in does not decide the
	// order the conversions run in.
	w.RegisterMigration(p.second)
	w.RegisterMigration(p.first)
}

// TestMigrateDeclaredByPlugin restores a save written by a plugin's first release into a world
// running its third.
//
// The point is that a plugin declares a migration through the same RegisterMigration a shard uses,
// and the result lands in the same registry, so a route is found the same way. Chaining is not a
// plugin feature — a shard can put its own component through as many versions — but a plugin is
// where it happens in practice, because a plugin author has no say in how old their users' saves
// are.
func TestMigrateDeclaredByPlugin(t *testing.T) {
	t.Parallel()

	// A shard running the plugin's first release, saving.
	source := ecs.NewWorld()
	_, err := source.RegisterComponent[early.Provided]()
	require.NoError(t, err)

	entity := source.Create()
	require.NoError(t, source.Set(entity, early.Provided{A: providedStored}))

	var save cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(source.EncodeState(nil), &save))

	// A shard running the plugin's third release, loading that save. The world is built directly
	// rather than through NewWorld: a migration needs no ticking, no storage and no NATS, so
	// anything more would be testing the parts around it.
	world := &World{world: ecs.NewWorld()}
	_, err = world.world.RegisterComponent[full.Provided]()
	require.NoError(t, err)

	plugin := &providedPlugin{first: &providedV1ToV2{}, second: &providedV2ToCurrent{}}
	world.RegisterPlugin(plugin)

	require.NoError(t, world.world.FromProto(&save))

	provided, err := world.world.Get[full.Provided](entity)
	require.NoError(t, err)
	assert.Equal(t, full.Provided{C: providedWantC}, provided)

	assert.Equal(t, 1, plugin.first.calls, "the first conversion must run once")
	assert.Equal(t, 1, plugin.second.calls, "the second conversion must run once")
}
