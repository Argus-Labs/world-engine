package cardinal

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	early "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/earlyaccess/component"
	full "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
	shard "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/migration"
	inventory "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/plugin/inventory/component"
	plugin "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/plugin/inventory/migration"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// The stored values and what the conversions make of them. Written out rather than derived, so a
// test computing its expectation from the code under test cannot agree with a bug.
const (
	renamedStored  uint32 = 7
	providedStored uint32 = 4
	providedWantC  uint32 = 15 // (4 + 1) * 3
)

// inventoryPlugin is the plugin's registration, declaring migrations for the component it owns.
type inventoryPlugin struct {
	first  *plugin.ProvidedV1ToV2
	second *plugin.ProvidedV2ToCurrent
}

func (p *inventoryPlugin) Register(w *World) {
	// The second step is registered first, to show the order a plugin registers in does not decide
	// the order the conversions run in.
	w.RegisterMigration(p.second)
	w.RegisterMigration(p.first)
}

// TestMigrateDeclaredByPlugin restores one snapshot holding both a shard's component and a
// plugin's, written by builds of each that are several releases behind.
//
// The shard and the plugin declare their migrations separately and neither knows the other exists.
// They land in one registry, and the engine routes by declared shape, so a plugin's conversion is
// indistinguishable from a shard's. The plugin's takes two steps because a plugin author has no say
// in how old their users' saves are; chaining is not a plugin feature, but this is where it happens.
func TestMigrateDeclaredByPlugin(t *testing.T) {
	t.Parallel()

	// The early-access shard, running the plugin's first release. One entity carries a component
	// from each, which is what a real save looks like.
	source := ecs.NewWorld()
	_, err := source.RegisterComponent[early.Renamed]()
	require.NoError(t, err)
	_, err = source.RegisterComponent[inventory.ProvidedV1]()
	require.NoError(t, err)

	entity := source.Create()
	require.NoError(t, source.Set(entity, early.Renamed{Before: renamedStored}))
	require.NoError(t, source.Set(entity, inventory.ProvidedV1{A: providedStored}))

	var save cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(source.EncodeState(nil), &save))

	// The full-release shard, running the plugin's third release. The world is built directly
	// rather than through NewWorld: a migration needs no ticking, no storage and no NATS, so
	// anything more would be testing the parts around it.
	world := &World{world: ecs.NewWorld()}
	_, err = world.world.RegisterComponent[full.Renamed]()
	require.NoError(t, err)
	_, err = world.world.RegisterComponent[inventory.Provided]()
	require.NoError(t, err)

	renamed := &shard.RenamedV1ToCurrent{}
	world.RegisterMigration(renamed)

	installed := &inventoryPlugin{first: &plugin.ProvidedV1ToV2{}, second: &plugin.ProvidedV2ToCurrent{}}
	world.RegisterPlugin(installed)

	require.NoError(t, world.world.FromProto(&save))

	// The shard's own component converted, in one step.
	gotRenamed, err := world.world.Get[full.Renamed](entity)
	require.NoError(t, err)
	assert.Equal(t, full.Renamed{After: renamedStored}, gotRenamed)
	assert.Equal(t, 1, renamed.Calls, "the shard's conversion must run once")

	// The plugin's component converted, in two, on the same entity in the same restore.
	gotProvided, err := world.world.Get[inventory.Provided](entity)
	require.NoError(t, err)
	assert.Equal(t, inventory.Provided{C: providedWantC}, gotProvided)
	assert.Equal(t, 1, installed.first.Calls, "the plugin's first conversion must run once")
	assert.Equal(t, 1, installed.second.Calls, "the plugin's second conversion must run once")
}
