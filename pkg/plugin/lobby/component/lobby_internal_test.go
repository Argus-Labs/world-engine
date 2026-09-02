package component

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newLobby builds a lobby through the same helpers production uses, so the roster and the per-team
// counts are consistent with each other. Constructing the struct literally would let a test assert
// against a state the real code can never produce.
func newLobby(t *testing.T, teams []Team, membership map[string][]string) *LobbyComponent {
	t.Helper()
	lobby := &LobbyComponent{}
	for _, team := range teams {
		require.True(t, lobby.AddTeam(team))
	}
	for _, team := range teams {
		for _, pid := range membership[team.TeamID] {
			require.True(t, lobby.AddPlayerToTeam(pid, team.TeamID))
		}
	}
	return lobby
}

func TestLobbyComponent_PlayerCount(t *testing.T) {
	t.Parallel()

	lobby := newLobby(t,
		[]Team{{TeamID: "team1"}, {TeamID: "team2"}},
		map[string][]string{"team1": {"p1", "p2"}, "team2": {"p3"}},
	)

	assert.Equal(t, 3, lobby.PlayerCount)
	assert.Equal(t, 2, lobby.GetTeam("team1").PlayerCount)
	assert.Equal(t, 1, lobby.GetTeam("team2").PlayerCount)
}

func TestLobbyComponent_HasPlayer(t *testing.T) {
	t.Parallel()

	lobby := newLobby(t, []Team{{TeamID: "team1"}}, map[string][]string{"team1": {"p1", "p2"}})

	assert.True(t, lobby.HasPlayer("p1"))
	assert.True(t, lobby.HasPlayer("p2"))
	assert.False(t, lobby.HasPlayer("p3"))
}

func TestLobbyComponent_GetTeam(t *testing.T) {
	t.Parallel()

	lobby := newLobby(t, []Team{{TeamID: "team1"}, {TeamID: "team2"}}, nil)

	team := lobby.GetTeam("team1")
	require.NotNil(t, team)
	assert.Equal(t, "team1", team.TeamID)

	assert.Nil(t, lobby.GetTeam("unknown"))
}

func TestLobbyComponent_AddTeamRejectsPastCap(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{}
	for i := range MaxLobbyTeams {
		require.True(t, lobby.AddTeam(Team{TeamID: "team" + strconv.Itoa(i)}))
	}
	assert.False(t, lobby.AddTeam(Team{TeamID: "one-too-many"}))
	assert.Equal(t, MaxLobbyTeams, lobby.TeamCount)
}

func TestLobbyComponent_IsLeader(t *testing.T) {
	t.Parallel()

	lobby := &LobbyComponent{LeaderID: "leader1"}

	assert.True(t, lobby.IsLeader("leader1"))
	assert.False(t, lobby.IsLeader("other"))
}

func TestLobbyComponent_AddPlayerToTeam(t *testing.T) {
	t.Parallel()

	lobby := newLobby(t,
		[]Team{{TeamID: "team1", MaxPlayers: 2}},
		map[string][]string{"team1": {"p1"}},
	)

	// Add player to existing team
	assert.True(t, lobby.AddPlayerToTeam("p2", "team1"))
	assert.True(t, lobby.HasPlayer("p2"))
	assert.Equal(t, 2, lobby.PlayerCount)

	// Try to add same player again
	assert.False(t, lobby.AddPlayerToTeam("p2", "team1"))

	// Try to add to non-existent team
	assert.False(t, lobby.AddPlayerToTeam("p3", "unknown"))

	// Try to add to full team
	assert.False(t, lobby.AddPlayerToTeam("p3", "team1"))
}

func TestLobbyComponent_AddPlayerRejectsPastRosterCap(t *testing.T) {
	t.Parallel()

	// An unlimited team (MaxPlayers 0) still cannot exceed the structural roster bound.
	lobby := newLobby(t, []Team{{TeamID: "team1", MaxPlayers: 0}}, nil)
	for i := range MaxLobbyPlayers {
		require.True(t, lobby.AddPlayerToTeam("p"+strconv.Itoa(i), "team1"))
	}

	assert.False(t, lobby.AddPlayerToTeam("one-too-many", "team1"))
	assert.Equal(t, MaxLobbyPlayers, lobby.PlayerCount)
	assert.Equal(t, MaxLobbyPlayers, lobby.GetTeam("team1").PlayerCount)
}

func TestLobbyComponent_RemovePlayerFromTeam(t *testing.T) {
	t.Parallel()

	lobby := newLobby(t,
		[]Team{{TeamID: "team1"}, {TeamID: "team2"}},
		map[string][]string{"team1": {"p1", "p2"}, "team2": {"p3"}},
	)

	// Remove player from their team
	assert.True(t, lobby.RemovePlayerFromTeam("p1", "team1"))
	assert.False(t, lobby.HasPlayer("p1"))
	assert.True(t, lobby.HasPlayer("p2"))
	assert.Equal(t, 2, lobby.PlayerCount)
	assert.Equal(t, 1, lobby.GetTeam("team1").PlayerCount)

	// Removing an unknown player changes nothing
	assert.False(t, lobby.RemovePlayerFromTeam("unknown", "team1"))
	assert.Equal(t, 2, lobby.PlayerCount)
}

// The roster keeps join order across removals because leader succession takes the first remaining
// entry. A swap-with-last removal would make the new leader arbitrary.
func TestLobbyComponent_RemovePreservesRosterOrder(t *testing.T) {
	t.Parallel()

	lobby := newLobby(t,
		[]Team{{TeamID: "team1"}},
		map[string][]string{"team1": {"p1", "p2", "p3", "p4"}},
	)

	require.True(t, lobby.RemovePlayerFromTeam("p2", "team1"))

	assert.Equal(t, []string{"p1", "p3", "p4"}, lobby.Players())
	// Vacated slots are cleared, not left holding a stale ID.
	assert.Empty(t, lobby.PlayerIDs[lobby.PlayerCount])
}

func TestLobbyComponent_MovePlayerToTeam(t *testing.T) {
	t.Parallel()

	lobby := newLobby(t,
		[]Team{{TeamID: "team1"}, {TeamID: "team2", MaxPlayers: 2}},
		map[string][]string{"team1": {"p1"}},
	)

	// Move player to another team
	assert.True(t, lobby.MovePlayerToTeam("p1", "team1", "team2"))
	assert.Equal(t, 0, lobby.GetTeam("team1").PlayerCount)
	assert.Equal(t, 1, lobby.GetTeam("team2").PlayerCount)
	// The lobby roster is unaffected by a team change.
	assert.Equal(t, 1, lobby.PlayerCount)

	// Move non-existent player
	assert.False(t, lobby.MovePlayerToTeam("unknown", "team1", "team1"))

	// Move to non-existent team
	assert.False(t, lobby.MovePlayerToTeam("p1", "team2", "unknown"))

	// Move to same team (no-op, should succeed)
	assert.True(t, lobby.MovePlayerToTeam("p1", "team2", "team2"))
	assert.Equal(t, 1, lobby.GetTeam("team2").PlayerCount)

	// Move to same team when at capacity (should succeed - player already there)
	lobby2 := newLobby(t,
		[]Team{{TeamID: "team1", MaxPlayers: 2}},
		map[string][]string{"team1": {"p1", "p2"}},
	)
	assert.True(t, lobby2.MovePlayerToTeam("p1", "team1", "team1"))
	assert.Equal(t, 2, lobby2.GetTeam("team1").PlayerCount)
}

func TestLobbyComponent_GetAllPlayerIDs(t *testing.T) {
	t.Parallel()

	lobby := newLobby(t,
		[]Team{{TeamID: "team1"}, {TeamID: "team2"}},
		map[string][]string{"team1": {"p1", "p2"}, "team2": {"p3"}},
	)

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
			team:     &Team{MaxPlayers: 0, PlayerCount: 3},
			expected: false,
		},
		{
			name:     "not full",
			team:     &Team{MaxPlayers: 3, PlayerCount: 2},
			expected: false,
		},
		{
			name:     "full",
			team:     &Team{MaxPlayers: 2, PlayerCount: 2},
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

func TestSessionStateConstants(t *testing.T) {
	t.Parallel()

	// Stable wire values for the session state machine. External consumers
	// (dashboards, audit logs) may persist these strings, so renames must
	// be intentional.
	assert.Equal(t, SessionStateIdle, SessionState("idle"))
	assert.Equal(t, SessionStateAwaitingAllocation, SessionState("awaiting_allocation"))
	assert.Equal(t, SessionStateInSession, SessionState("in_session"))
}

func TestSessionPendingFields(t *testing.T) {
	t.Parallel()

	// Session carries the correlation fields the AssignShardCommand
	// handler validates against. Renaming or removing these would silently
	// break orchestrators that echo PendingRequestID.
	s := Session{
		State:            SessionStateAwaitingAllocation,
		PendingRequestID: "req-42",
		PendingStartedAt: 100,
	}
	assert.Equal(t, SessionStateAwaitingAllocation, s.State)
	assert.Equal(t, "req-42", s.PendingRequestID)
	assert.Equal(t, int64(100), s.PendingStartedAt)
}

// A snapshot written when the caps were larger restores counts the arrays cannot satisfy: the
// generated decoder caps its array writes but copies PlayerCount and TeamCount verbatim. Reads must
// truncate rather than panic on a slice bound.
func TestLobbyComponent_ReadsSurviveOversizedRestoredCounts(t *testing.T) {
	t.Parallel()

	lobby := newLobby(t, []Team{{TeamID: "team1"}}, map[string][]string{"team1": {"p1", "p2"}})
	// Simulate the restore: counts beyond what the fixed arrays hold.
	lobby.PlayerCount = MaxLobbyPlayers + 4
	lobby.TeamCount = MaxLobbyTeams + 2

	assert.NotPanics(t, func() {
		assert.Len(t, lobby.Players(), MaxLobbyPlayers)
		assert.Len(t, lobby.GetAllPlayerIDs(), MaxLobbyPlayers)
		assert.True(t, lobby.HasPlayer("p1"))
		assert.NotNil(t, lobby.GetTeam("team1"))
		assert.Nil(t, lobby.GetTeam("nope"))
	})
}

// The first removal writes the clamped count back, so an oversized count repairs itself rather than
// persisting through every later read.
func TestLobbyComponent_RemoveRepairsOversizedCount(t *testing.T) {
	t.Parallel()

	lobby := newLobby(t, []Team{{TeamID: "team1"}}, map[string][]string{"team1": {"p1", "p2"}})
	lobby.PlayerCount = MaxLobbyPlayers + 4

	require.True(t, lobby.RemovePlayerFromTeam("p1", "team1"))
	assert.Equal(t, MaxLobbyPlayers-1, lobby.PlayerCount)
	assert.False(t, lobby.HasPlayer("p1"))
}
