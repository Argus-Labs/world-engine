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
			{TeamID: "team1", Players: []PlayerState{{PlayerID: "p1"}, {PlayerID: "p2"}}},
			{TeamID: "team2", Players: []PlayerState{{PlayerID: "p3"}}},
		},
	}

	assert.Equal(t, 3, lobby.PlayerCount())
}

func TestLobbyComponent_HasPlayer(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", Players: []PlayerState{{PlayerID: "p1"}, {PlayerID: "p2"}}},
		},
	}

	assert.True(t, lobby.HasPlayer("p1"))
	assert.True(t, lobby.HasPlayer("p2"))
	assert.False(t, lobby.HasPlayer("p3"))
}

func TestLobbyComponent_GetPlayer(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", Players: []PlayerState{{PlayerID: "p1", IsReady: true}}},
		},
	}

	player := lobby.GetPlayer("p1")
	require.NotNil(t, player)
	assert.Equal(t, "p1", player.PlayerID)
	assert.True(t, player.IsReady)

	assert.Nil(t, lobby.GetPlayer("unknown"))
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
			{TeamID: "team1", Players: []PlayerState{{PlayerID: "p1"}}},
			{TeamID: "team2", Players: []PlayerState{{PlayerID: "p2"}}},
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

func TestLobbyComponent_AllReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lobby    *LobbyComponent
		expected bool
	}{
		{
			name: "all ready",
			lobby: &LobbyComponent{
				Teams: []Team{
					{Players: []PlayerState{{PlayerID: "p1", IsReady: true}, {PlayerID: "p2", IsReady: true}}},
				},
			},
			expected: true,
		},
		{
			name: "not all ready",
			lobby: &LobbyComponent{
				Teams: []Team{
					{Players: []PlayerState{{PlayerID: "p1", IsReady: true}, {PlayerID: "p2", IsReady: false}}},
				},
			},
			expected: false,
		},
		{
			name: "empty lobby",
			lobby: &LobbyComponent{
				Teams: []Team{},
			},
			expected: false,
		},
		{
			name: "empty teams",
			lobby: &LobbyComponent{
				Teams: []Team{{Players: []PlayerState{}}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.lobby.AllReady())
		})
	}
}

func TestLobbyComponent_AddPlayerToTeam(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", MaxPlayers: 2, Players: []PlayerState{{PlayerID: "p1"}}},
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
			{TeamID: "team1", Players: []PlayerState{{PlayerID: "p1"}, {PlayerID: "p2"}}},
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

func TestLobbyComponent_SetReady(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", Players: []PlayerState{{PlayerID: "p1", IsReady: false}}},
		},
	}

	lobby.SetReady("p1", true)
	assert.True(t, lobby.GetPlayer("p1").IsReady)

	lobby.SetReady("p1", false)
	assert.False(t, lobby.GetPlayer("p1").IsReady)

	// Set ready for non-existent player (should not panic)
	lobby.SetReady("unknown", true)
}

func TestLobbyComponent_MovePlayerToTeam(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{
		Teams: []Team{
			{TeamID: "team1", Players: []PlayerState{{PlayerID: "p1", IsReady: true}}},
			{TeamID: "team2", MaxPlayers: 2, Players: []PlayerState{}},
		},
	}

	// Move player to another team
	assert.True(t, lobby.MovePlayerToTeam("p1", "team2"))
	assert.Empty(t, lobby.GetTeam("team1").Players)
	assert.Equal(t, 1, len(lobby.GetTeam("team2").Players))

	// Verify player state is preserved
	player := lobby.GetPlayer("p1")
	require.NotNil(t, player)
	assert.True(t, player.IsReady)

	// Move non-existent player
	assert.False(t, lobby.MovePlayerToTeam("unknown", "team1"))

	// Move to non-existent team
	assert.False(t, lobby.MovePlayerToTeam("p1", "unknown"))
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
			team:     &Team{MaxPlayers: 0, Players: []PlayerState{{}, {}, {}}},
			expected: false,
		},
		{
			name:     "not full",
			team:     &Team{MaxPlayers: 3, Players: []PlayerState{{}, {}}},
			expected: false,
		},
		{
			name:     "full",
			team:     &Team{MaxPlayers: 2, Players: []PlayerState{{}, {}}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.team.IsFull())
		})
	}
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

	// Add player to lobby
	idx.AddPlayerToLobby("player1", "lobby1")

	lobbyID, exists := idx.GetPlayerLobby("player1")
	assert.True(t, exists)
	assert.Equal(t, "lobby1", lobbyID)

	// Remove player
	idx.RemovePlayerFromLobby("player1")

	_, exists = idx.GetPlayerLobby("player1")
	assert.False(t, exists)
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
