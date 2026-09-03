package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/plugin/lobby/component"
)

func TestLookupIndex_AddRemoveLobby(t *testing.T) {
	t.Parallel()

	idx := &lookupIndex{}
	idx.Init()

	// Add lobby
	idx.AddLobby("lobby1", 100, "ABC123")

	entityID, exists := idx.GetEntityID("lobby1")
	assert.True(t, exists)
	assert.Equal(t, uint32(100), entityID)

	lobbyID, exists := idx.GetLobbyByInviteCode("ABC123")
	assert.True(t, exists)
	assert.Equal(t, "lobby1", lobbyID)

	// Remove lobby
	idx.RemoveLobby("lobby1", "ABC123")

	_, exists = idx.GetEntityID("lobby1")
	assert.False(t, exists)

	_, exists = idx.GetLobbyByInviteCode("ABC123")
	assert.False(t, exists)
}

func TestLookupIndex_PlayerToLobby(t *testing.T) {
	t.Parallel()

	idx := &lookupIndex{}
	idx.Init()

	deadline := int64(1000)
	playerEntityID := uint32(200)

	// Add player to lobby with team
	idx.AddPlayerToLobby("player1", "lobby1", "team1", playerEntityID, deadline)

	lobbyID, exists := idx.GetPlayerLobby("player1")
	assert.True(t, exists)
	assert.Equal(t, "lobby1", lobbyID)

	// Verify player team ID
	teamID, exists := idx.GetPlayerTeam("player1")
	assert.True(t, exists)
	assert.Equal(t, "team1", teamID)

	// Verify player entity ID
	entityID, exists := idx.GetPlayerEntityID("player1")
	assert.True(t, exists)
	assert.Equal(t, playerEntityID, entityID)

	// Verify deadline was initialized
	playerDeadline, exists := idx.GetPlayerDeadline("player1")
	assert.True(t, exists)
	assert.Equal(t, deadline, playerDeadline)

	// Verify lobby player count
	assert.Equal(t, 1, idx.GetLobbyPlayerCount("lobby1"))

	// Update deadline
	newDeadline := int64(2000)
	idx.UpdatePlayerDeadline("player1", newDeadline)
	playerDeadline, _ = idx.GetPlayerDeadline("player1")
	assert.Equal(t, newDeadline, playerDeadline)

	// Update team
	idx.UpdatePlayerTeam("player1", "team2")
	teamID, _ = idx.GetPlayerTeam("player1")
	assert.Equal(t, "team2", teamID)

	// Remove player
	idx.RemovePlayerFromLobby("player1")

	_, exists = idx.GetPlayerLobby("player1")
	assert.False(t, exists)

	// Verify player team was also removed
	_, exists = idx.GetPlayerTeam("player1")
	assert.False(t, exists)

	// Verify player entity ID was also removed
	_, exists = idx.GetPlayerEntityID("player1")
	assert.False(t, exists)

	// Verify deadline was also removed
	_, exists = idx.GetPlayerDeadline("player1")
	assert.False(t, exists)

	// Verify lobby player count is 0
	assert.Equal(t, 0, idx.GetLobbyPlayerCount("lobby1"))
}

func TestLookupIndex_HasPlayer(t *testing.T) {
	t.Parallel()

	idx := &lookupIndex{}
	idx.Init()

	assert.False(t, idx.HasPlayer("player1"))

	idx.AddPlayerToLobby("player1", "lobby1", "team1", 100, 1000)
	assert.True(t, idx.HasPlayer("player1"))

	idx.RemovePlayerFromLobby("player1")
	assert.False(t, idx.HasPlayer("player1"))
}

func TestLookupIndex_LobbyPlayerCount(t *testing.T) {
	t.Parallel()

	idx := &lookupIndex{}
	idx.Init()

	// Add players to lobby
	idx.AddPlayerToLobby("p1", "lobby1", "team1", 100, 1000)
	idx.AddPlayerToLobby("p2", "lobby1", "team1", 101, 1000)
	idx.AddPlayerToLobby("p3", "lobby1", "team2", 102, 1000)

	assert.Equal(t, 3, idx.GetLobbyPlayerCount("lobby1"))

	// Remove one player
	idx.RemovePlayerFromLobby("p2")
	assert.Equal(t, 2, idx.GetLobbyPlayerCount("lobby1"))

	// Remove remaining players
	idx.RemovePlayerFromLobby("p1")
	idx.RemovePlayerFromLobby("p3")
	assert.Equal(t, 0, idx.GetLobbyPlayerCount("lobby1"))
}

func TestLookupIndex_UpdateInviteCode(t *testing.T) {
	t.Parallel()

	idx := &lookupIndex{}
	idx.Init()

	// Add lobby with invite code
	idx.AddLobby("lobby1", 100, "OLD123")

	// Update invite code
	idx.UpdateInviteCode("lobby1", "OLD123", "NEW456")

	// Old code should not work
	_, exists := idx.GetLobbyByInviteCode("OLD123")
	assert.False(t, exists)

	// New code should work
	lobbyID, exists := idx.GetLobbyByInviteCode("NEW456")
	assert.True(t, exists)
	assert.Equal(t, "lobby1", lobbyID)
}

// rebuildIndex writes the package-level index, so this cannot run in parallel with anything else
// that touches it.
func TestRebuildIndexFromEntities(t *testing.T) {
	t.Cleanup(func() {
		index = lookupIndex{}
		indexBuilt = false
	})

	lobbies := []lobbyRow{
		{entityID: 7, lobby: component.LobbyComponent{ID: "lobby1", InviteCode: "ABC123"}},
	}
	players := []playerRow{
		{entityID: 11, player: component.PlayerComponent{PlayerID: "p1", LobbyID: "lobby1", TeamID: "team1"}},
		{entityID: 12, player: component.PlayerComponent{PlayerID: "p2", LobbyID: "lobby1", TeamID: "team1"}},
	}

	rebuildIndex(lobbies, players, 1000, 30)

	require.True(t, indexBuilt)

	entityID, exists := index.GetEntityID("lobby1")
	assert.True(t, exists)
	assert.Equal(t, uint32(7), entityID)

	lobbyID, exists := index.GetLobbyByInviteCode("ABC123")
	assert.True(t, exists)
	assert.Equal(t, "lobby1", lobbyID)

	teamID, exists := index.GetPlayerTeam("p1")
	assert.True(t, exists)
	assert.Equal(t, "team1", teamID)

	playerEntityID, exists := index.GetPlayerEntityID("p2")
	assert.True(t, exists)
	assert.Equal(t, uint32(12), playerEntityID)

	assert.Equal(t, 2, index.GetLobbyPlayerCount("lobby1"))
}

// Deadlines are recomputed from the rebuild's clock, never carried over. A restored shard whose
// downtime exceeded the heartbeat timeout would otherwise evict every player on its first tick.
func TestRebuildIndexResetsDeadlines(t *testing.T) {
	t.Cleanup(func() {
		index = lookupIndex{}
		indexBuilt = false
	})

	const (
		now     = int64(50_000)
		timeout = int64(30)
	)
	players := []playerRow{
		{entityID: 1, player: component.PlayerComponent{PlayerID: "p1", LobbyID: "lobby1"}},
	}

	rebuildIndex(nil, players, now, timeout)

	deadline, exists := index.GetPlayerDeadline("p1")
	require.True(t, exists)
	assert.Equal(t, now+timeout, deadline)
	assert.Greater(t, deadline, now, "a rebuilt deadline must be in the future")
}

// A rebuild replaces the index outright rather than merging into whatever was there.
func TestRebuildIndexDiscardsPriorState(t *testing.T) {
	t.Cleanup(func() {
		index = lookupIndex{}
		indexBuilt = false
	})

	index.Init()
	index.AddLobby("stale", 1, "GONE99")

	rebuildIndex(nil, nil, 1000, 30)

	_, exists := index.GetEntityID("stale")
	assert.False(t, exists)
	_, exists = index.GetLobbyByInviteCode("GONE99")
	assert.False(t, exists)
}
