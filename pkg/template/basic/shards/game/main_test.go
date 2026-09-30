package main_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	otherworld "github.com/argus-labs/world-engine/pkg/template/basic/pkg/other_worlds"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/event"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system"
	systemevent "github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system_event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDST(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		registerSystems(w)
	}, nil)
}

func TestGraveyardSystem(t *testing.T) {
	t.Parallel()

	// registerSystems also runs PlayerSpawnerSystem at init, so the world starts with 10 players.
	w := cardinal.NewTestWorld(t, registerSystems)
	w.EmitSystemEvent(systemevent.PlayerDeath{Nickname: "bob"})

	w.RunSystem(&system.GraveyardSystem{})

	var graves []component.Gravestone
	for grave := range w.Exact[system.Grave]().Iter() {
		graves = append(graves, grave.Get[component.Gravestone]())
	}
	assert.Equal(t, []component.Gravestone{{Nickname: "bob"}}, graves)
}

func TestE2E(t *testing.T) {
	cardinal.RunE2E(t, func() *cardinal.World {
		debug := false

		// Keep world setup aligned with shards/game/main.go.
		w, err := cardinal.NewWorld(cardinal.WorldOptions{
			Region:              "local",
			Organization:        "organization",
			Project:             "project",
			ShardID:             "game",
			TickRate:            1,
			SnapshotRate:        50,
			SnapshotStorageType: snapshot.StorageTypeJetStream,
			Debug:               &debug,
		})
		require.NoError(t, err)

		registerSystems(w)

		return w
	})
}

func registerSystems(w *cardinal.World) {
	w.RegisterComponent[component.PlayerTag]()
	w.RegisterComponent[component.Health]()
	w.RegisterComponent[component.Gravestone]()

	w.RegisterCommand[system.CreatePlayerCommand]()
	w.RegisterCommand[system.AttackPlayerCommand]()
	w.RegisterCommand[system.CallExternalCommand]()

	w.RegisterEvent[event.NewPlayer]()
	w.RegisterEvent[event.PlayerDeath]()

	w.RegisterSystemEvent[systemevent.PlayerDeath]()

	w.RegisterSystem(&system.PlayerSpawnerSystem{}, cardinal.WithHook(cardinal.Init))
	w.RegisterSystem(&system.CreatePlayerSystem{})
	w.RegisterSystem(&system.RegenSystem{})
	w.RegisterSystem(&system.AttackPlayerSystem{})
	w.RegisterSystem(&system.GraveyardSystem{})
	w.RegisterSystem(&system.CallExternalSystem{MatchmakingWorld: otherworld.Matchmaking()})
}
