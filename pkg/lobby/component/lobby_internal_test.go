package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLobbyComponent_PlayerCount(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", PlayerIDs: []string{"p1", "p2"}},
			{TeamID: "team2", PlayerIDs: []string{"p3"}},
		},
	}

	assert.Equal(t, 3, lobby.PlayerCount())
}

func TestLobbyComponent_HasPlayer(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", PlayerIDs: []string{"p1", "p2"}},
		},
	}

	assert.True(t, lobby.HasPlayer("p1"))
	assert.True(t, lobby.HasPlayer("p2"))
	assert.False(t, lobby.HasPlayer("p3"))
}

func TestLobbyComponent_GetTeam(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", Name: "Team One"},
			{TeamID: "team2", Name: "Team Two"},
		},
	}

	team := lobby.GetTeam("team1")
	require.NotNil(t, team)
	assert.Equal(t, "Team One", team.Name)

	assert.Nil(t, lobby.GetTeam("unknown"))
}

func TestLobbyComponent_GetPlayerTeam(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", PlayerIDs: []string{"p1"}},
			{TeamID: "team2", PlayerIDs: []string{"p2"}},
		},
	}

	team := lobby.GetPlayerTeam("p1")
	require.NotNil(t, team)
	assert.Equal(t, "team1", team.TeamID)

	team = lobby.GetPlayerTeam("p2")
	require.NotNil(t, team)
	assert.Equal(t, "team2", team.TeamID)

	assert.Nil(t, lobby.GetPlayerTeam("unknown"))
}

func TestLobbyComponent_IsLeader(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{LeaderID: "leader1"}

	assert.True(t, lobby.IsLeader("leader1"))
	assert.False(t, lobby.IsLeader("other"))
}

func TestLobbyComponent_AddPlayerToTeam(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", MaxPlayers: 2, PlayerIDs: []string{"p1"}},
		},
	}

	// Add player to existing team
	assert.True(t, lobby.AddPlayerToTeam("p2", "team1"))
	assert.True(t, lobby.HasPlayer("p2"))
	assert.Equal(t, 2, lobby.PlayerCount())

	// Try to add same player again
	assert.False(t, lobby.AddPlayerToTeam("p2", "team1"))

	// Try to add to non-existent team
	assert.False(t, lobby.AddPlayerToTeam("p3", "unknown"))

	// Try to add to full team
	assert.False(t, lobby.AddPlayerToTeam("p3", "team1"))
}

func TestLobbyComponent_RemovePlayer(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", PlayerIDs: []string{"p1", "p2"}},
		},
	}

	lobby.RemovePlayer("p1")
	assert.False(t, lobby.HasPlayer("p1"))
	assert.True(t, lobby.HasPlayer("p2"))
	assert.Equal(t, 1, lobby.PlayerCount())

	// Remove non-existent player (should not panic)
	lobby.RemovePlayer("unknown")
	assert.Equal(t, 1, lobby.PlayerCount())
}

func TestLobbyComponent_RemovePlayerFromTeam(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", PlayerIDs: []string{"p1", "p2"}},
			{TeamID: "team2", PlayerIDs: []string{"p3"}},
		},
	}

	// Remove player from correct team
	assert.True(t, lobby.RemovePlayerFromTeam("p1", "team1"))
	assert.False(t, lobby.HasPlayer("p1"))
	assert.True(t, lobby.HasPlayer("p2"))
	assert.Equal(t, 2, lobby.PlayerCount())

	// Try to remove player from wrong team
	assert.False(t, lobby.RemovePlayerFromTeam("p2", "team2"))
	assert.True(t, lobby.HasPlayer("p2")) // Still exists

	// Try to remove from non-existent team
	assert.False(t, lobby.RemovePlayerFromTeam("p2", "unknown"))
	assert.True(t, lobby.HasPlayer("p2"))

	// Try to remove non-existent player from valid team
	assert.False(t, lobby.RemovePlayerFromTeam("unknown", "team1"))
}

func TestLobbyComponent_MovePlayerToTeam(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", PlayerIDs: []string{"p1"}},
			{TeamID: "team2", MaxPlayers: 2, PlayerIDs: []string{}},
		},
	}

	// Move player to another team
	assert.True(t, lobby.MovePlayerToTeam("p1", "team2"))
	assert.Empty(t, lobby.GetTeam("team1").PlayerIDs)
	assert.Len(t, lobby.GetTeam("team2").PlayerIDs, 1)

	// Move non-existent player
	assert.False(t, lobby.MovePlayerToTeam("unknown", "team1"))

	// Move to non-existent team
	assert.False(t, lobby.MovePlayerToTeam("p1", "unknown"))

	// Move to same team (no-op, should succeed)
	assert.True(t, lobby.MovePlayerToTeam("p1", "team2"))
	assert.Len(t, lobby.GetTeam("team2").PlayerIDs, 1)

	// Move to same team when at capacity (should succeed - player already there)
	lobby2 := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", MaxPlayers: 2, PlayerIDs: []string{"p1", "p2"}},
		},
	}
	assert.True(t, lobby2.MovePlayerToTeam("p1", "team1"))
	assert.Equal(t, []string{"p1", "p2"}, lobby2.GetTeam("team1").PlayerIDs)
}

func TestLobbyComponent_GetAllPlayerIDs(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", PlayerIDs: []string{"p1", "p2"}},
			{TeamID: "team2", PlayerIDs: []string{"p3"}},
		},
	}

	playerIDs := lobby.GetAllPlayerIDs()
	assert.Len(t, playerIDs, 3)
	assert.Contains(t, playerIDs, "p1")
	assert.Contains(t, playerIDs, "p2")
	assert.Contains(t, playerIDs, "p3")
}

func TestTeam_IsFull(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		team     *Team
		expected bool
	}{
		{
			name:     "unlimited team",
			team:     &Team{MaxPlayers: 0, PlayerIDs: []string{"p1", "p2", "p3"}},
			expected: false,
		},
		{
			name:     "not full",
			team:     &Team{MaxPlayers: 3, PlayerIDs: []string{"p1", "p2"}},
			expected: false,
		},
		{
			name:     "full",
			team:     &Team{MaxPlayers: 2, PlayerIDs: []string{"p1", "p2"}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.team.IsFull())
		})
	}
}

func TestPlayerComponent_Name(t *testing.T) {
	t.Parallel()

	player := PlayerComponent{}
	assert.Equal(t, "player", player.Name())
}

func TestLobbyIndexComponent_AddRemoveLobby(t *testing.T) {
	t.Parallel()

	idx := &LobbyIndexComponent{}
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

func TestLobbyIndexComponent_PlayerToLobby(t *testing.T) {
	t.Parallel()

	idx := &LobbyIndexComponent{}
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

func TestLobbyIndexComponent_HasPlayer(t *testing.T) {
	t.Parallel()

	idx := &LobbyIndexComponent{}
	idx.Init()

	assert.False(t, idx.HasPlayer("player1"))

	idx.AddPlayerToLobby("player1", "lobby1", "team1", 100, 1000)
	assert.True(t, idx.HasPlayer("player1"))

	idx.RemovePlayerFromLobby("player1")
	assert.False(t, idx.HasPlayer("player1"))
}

func TestLobbyIndexComponent_LobbyPlayerCount(t *testing.T) {
	t.Parallel()

	idx := &LobbyIndexComponent{}
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

func TestLobbyIndexComponent_UpdateInviteCode(t *testing.T) {
	t.Parallel()

	idx := &LobbyIndexComponent{}
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
