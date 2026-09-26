package lobby_test

import (
	"context"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/plugin/lobby"
	"github.com/argus-labs/world-engine/pkg/plugin/lobby/system"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type ownershipProvider struct {
	code      string
	allowJoin bool
}

func (p ownershipProvider) GenerateInviteCode(*lobby.Component, int64) string { return p.code }
func (p ownershipProvider) ValidateJoin(*lobby.Component, lobby.JoinLobbyCommand) (bool, string) {
	return p.allowJoin, "join rejected by this world's provider"
}

// Match Cardinal's real startup/reset/restore lifecycle without starting network services.
// This is the same access pattern as the physics and data integration tests.
type ownershipECS interface {
	Init()
	Reset()
	EncodeState([]byte) []byte
	FromProto(*cardinalv1.WorldState) error
}

type ownershipCommands interface {
	Enqueue(context.Context, *iscv1.Command) error
}

type ownershipWorld struct {
	world    *cardinal.World
	ecs      ownershipECS
	commands ownershipCommands
}

func newOwnershipWorld(t *testing.T, config lobby.Config) *ownershipWorld {
	t.Helper()
	return newOwnershipWorldWithPlugin(t, lobby.NewPlugin(config))
}

func newOwnershipWorldWithPlugin(t *testing.T, plugin *lobby.Plugin) *ownershipWorld {
	t.Helper()
	return newOwnershipWorldWithSetup(t, func(w *cardinal.World) { w.RegisterPlugin(plugin) })
}

// newOwnershipWorldWithSetup builds an initialized world, running setup before initialization so
// it can register plugins or systems.
func newOwnershipWorldWithSetup(t *testing.T, setup func(*cardinal.World)) *ownershipWorld {
	t.Helper()
	debug := false
	world, err := cardinal.NewWorld(cardinal.WorldOptions{
		Region: "local", Organization: "lobby-test", Project: "lobby-test", ShardID: "0",
		TickRate: 1, SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate: 1_000_000, Debug: &debug,
	})
	require.NoError(t, err)
	setup(world)
	value := reflect.ValueOf(world).Elem()
	ecsField := value.FieldByName("world")
	ecsValue := reflect.NewAt(ecsField.Type(), unsafe.Pointer(ecsField.UnsafeAddr())).Elem()
	ecs, ok := ecsValue.Interface().(ownershipECS)
	require.True(t, ok)
	commandField := value.FieldByName("commands")
	commandValue := reflect.NewAt(commandField.Type(), unsafe.Pointer(commandField.UnsafeAddr())).Interface()
	commands, ok := commandValue.(ownershipCommands)
	require.True(t, ok)
	ecs.Init()
	return &ownershipWorld{world: world, ecs: ecs, commands: commands}
}

func (w *ownershipWorld) send(t *testing.T, persona string, payload interface {
	Name() string
	MarshalWire() []byte
}) {
	t.Helper()
	require.NoError(t, w.commands.Enqueue(context.Background(), &iscv1.Command{
		Name: payload.Name(), Address: &microv1.ServiceAddress{},
		Persona: &iscv1.Persona{Id: persona}, Payload: payload.MarshalWire(),
	}))
}

func (w *ownershipWorld) tick(timestamp int64) { w.world.Tick(time.Unix(timestamp, 0)) }

func (w *ownershipWorld) lobbies() []lobby.Component {
	var rows []lobby.Component
	for entity := range w.world.Contains[struct{ Lobby lobby.Component }]().Iter() {
		rows = append(rows, entity.Get[lobby.Component]())
	}
	return rows
}

func (w *ownershipWorld) onlyLobby(t *testing.T) lobby.Component {
	t.Helper()
	rows := w.lobbies()
	require.Len(t, rows, 1)
	return rows[0]
}

func TestPluginWorldsOwnConfigProviderAndIndex(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	presetsA := map[string][]lobby.TeamConfig{"duo": {{TeamID: "red", MaxPlayers: 2}}}
	a := newOwnershipWorld(t, lobby.Config{
		HeartbeatTimeout: 5, LobbyPresets: presetsA,
		Provider: ownershipProvider{code: "ALPHA", allowJoin: true},
	})
	a.send(t, "leader", lobby.CreateLobbyCommand{RequestID: "create-a", Preset: "duo"})
	a.tick(100)
	assert.Equal(t, "ALPHA", a.onlyLobby(t).InviteCode)

	b := newOwnershipWorld(t, lobby.Config{
		HeartbeatTimeout: 50,
		LobbyPresets:     map[string][]lobby.TeamConfig{"duo": {{TeamID: "blue", MaxPlayers: 4}}},
		Provider:         ownershipProvider{code: "BETA", allowJoin: false},
	})
	b.send(t, "leader", lobby.CreateLobbyCommand{RequestID: "create-b", Preset: "duo"})
	b.tick(100)
	assert.Equal(t, "BETA", b.onlyLobby(t).InviteCode)
	a.send(t, "friend", lobby.JoinLobbyCommand{RequestID: "join-a", InviteCode: "ALPHA"})
	b.send(t, "friend", lobby.JoinLobbyCommand{RequestID: "join-b", InviteCode: "BETA"})
	a.tick(101)
	b.tick(101)
	assert.Equal(t, 2, a.onlyLobby(t).PlayerCount)
	assert.Equal(t, 1, b.onlyLobby(t).PlayerCount)
	assert.Equal(t, "red", a.onlyLobby(t).Teams[0].TeamID)
	assert.Equal(t, "blue", b.onlyLobby(t).Teams[0].TeamID)

	// A reset must not renew B's heartbeat deadline by invalidating B's lookup index.
	a.ecs.Reset()
	a.ecs.Init()
	// The runtime copied both the map and the nested team slice at registration.
	presetsA["duo"][0].TeamID = "mutated"
	delete(presetsA, "duo")
	a.send(t, "leader", lobby.CreateLobbyCommand{RequestID: "recreate-a", Preset: "duo"})
	a.tick(150)
	b.tick(151)
	a.tick(153)
	assert.Empty(t, b.lobbies(), "B must expire at its original deadline")
	surviving := a.onlyLobby(t)
	assert.Equal(t, "ALPHA", surviving.InviteCode)
	assert.Equal(t, "red", surviving.Teams[0].TeamID)
	assert.Equal(t, 1, surviving.PlayerCount)
}

func TestPluginRestoreRebuildsOnlyItsOwnIndex(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	config := lobby.Config{
		HeartbeatTimeout: 5,
		LobbyPresets:     map[string][]lobby.TeamConfig{"duo": {{TeamID: "red", MaxPlayers: 2}}},
		Provider:         ownershipProvider{code: "RESTORE", allowJoin: true},
	}
	original := newOwnershipWorld(t, config)
	original.send(t, "leader", lobby.CreateLobbyCommand{RequestID: "create", Preset: "duo"})
	original.tick(100)
	var saved cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(original.ecs.EncodeState(nil), &saved))

	// Init precedes restore in the shard loop. The first tick must rebuild from restored entities.
	restored := newOwnershipWorld(t, config)
	require.NoError(t, restored.ecs.FromProto(&saved))
	restored.send(t, "friend", lobby.JoinLobbyCommand{RequestID: "join", InviteCode: "RESTORE"})
	restored.tick(1_000)
	assert.Equal(t, 2, restored.onlyLobby(t).PlayerCount)
	original.tick(106)
	assert.Empty(t, original.lobbies())
	restored.tick(1_004)
	assert.Equal(t, 2, restored.onlyLobby(t).PlayerCount)
	restored.tick(1_005)
	assert.Empty(t, restored.lobbies())
}

func TestPluginNilProviderUsesItsOwnDefault(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	presets := map[string][]lobby.TeamConfig{"solo": {{TeamID: "solo", MaxPlayers: 1}}}
	custom := newOwnershipWorld(t, lobby.Config{
		LobbyPresets: presets, Provider: ownershipProvider{code: "CUSTOM"},
	})
	defaults := newOwnershipWorld(t, lobby.Config{LobbyPresets: presets})
	custom.send(t, "leader", lobby.CreateLobbyCommand{RequestID: "custom", Preset: "solo"})
	defaults.send(t, "leader", lobby.CreateLobbyCommand{RequestID: "default", Preset: "solo"})
	custom.tick(100)
	defaults.tick(100)
	assert.Equal(t, "CUSTOM", custom.onlyLobby(t).InviteCode)
	created := defaults.onlyLobby(t)
	expected := (lobby.DefaultProvider{}).GenerateInviteCode(&created, time.Unix(100, 0).UnixNano())
	assert.Equal(t, expected, created.InviteCode)
	assert.NotEqual(t, "CUSTOM", created.InviteCode)
}

func TestPluginRejectsReuse(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	plugin := lobby.NewPlugin(lobby.Config{})
	first := newOwnershipWorldWithPlugin(t, plugin)
	second := newOwnershipWorld(t, lobby.Config{})
	first.tick(100)
	assert.PanicsWithValue(t,
		"lobby: Plugin.Register called twice on the same instance; create a separate plugin instance per world",
		func() { second.world.RegisterPlugin(plugin) },
	)
}

func TestZeroValueSystemsPanicWithoutRuntime(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	message := func(name string) string {
		return "lobby: " + name + " has no runtime; register lobby.NewPlugin instead of constructing the system directly"
	}
	// Init-hook systems run while the world initializes.
	require.PanicsWithValue(t, message("InitSystem"), func() {
		newOwnershipWorldWithSetup(t, func(w *cardinal.World) {
			w.RegisterSystem(&system.InitSystem{}, cardinal.WithHook(cardinal.Init))
		})
	})
	for name, sys := range map[string]cardinal.System{
		"LobbySystem":     &system.LobbySystem{},
		"HeartbeatSystem": &system.HeartbeatSystem{},
	} {
		world := newOwnershipWorldWithSetup(t, func(w *cardinal.World) { w.RegisterSystem(sys) })
		require.PanicsWithValue(t, message(name), func() { world.tick(100) })
	}
}
