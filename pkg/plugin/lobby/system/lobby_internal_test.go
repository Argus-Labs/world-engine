package system

import (
	"strconv"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/lobby/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultProvider_GenerateInviteCode checks the generator against its spec:
// codes are inviteCodeLength characters drawn from inviteCodeCharset, and every
// character in the charset is actually usable. Deterministic — the generator is
// pure in (lobby, seed), so the seeds are fixed and no clock is involved.
func TestDefaultProvider_GenerateInviteCode(t *testing.T) {
	t.Parallel()

	provider := DefaultProvider{}
	lobby := &component.LobbyComponent{ID: "test-lobby-id"}

	// Enough seeds that every charset character has occurred; the set below is the
	// exact output, so a generator that cannot reach all 31 characters fails here.
	used := make(map[rune]bool, len(inviteCodeCharset))
	for seed := range int64(500) {
		code := provider.GenerateInviteCode(lobby, seed)

		assert.Len(t, code, inviteCodeLength)
		for _, c := range code {
			assert.Contains(t, inviteCodeCharset, string(c), "seed %d produced invalid character %q", seed, c)
			used[c] = true
		}
	}

	for _, c := range inviteCodeCharset {
		assert.True(t, used[c], "generator never emits %q; the charset is not fully usable", c)
	}
}

// TestDefaultProvider_GenerateInviteCode_IsPure pins the contract the retry loop
// depends on: same inputs give the same code, and a different seed gives a
// different one. Without the latter, generateInviteCodeWithRetry could never
// escape a collision.
func TestDefaultProvider_GenerateInviteCode_IsPure(t *testing.T) {
	t.Parallel()

	provider := DefaultProvider{}
	lobby := &component.LobbyComponent{ID: "test-lobby-id"}

	// Same seed, same code.
	sameSeedA := provider.GenerateInviteCode(lobby, 42)
	sameSeedB := provider.GenerateInviteCode(lobby, 42)
	assert.Equal(t, sameSeedA, sameSeedB)

	// Consecutive seeds are what generateInviteCodeWithRetry feeds each attempt,
	// so they must differ or a collision could never be escaped.
	nextSeed := provider.GenerateInviteCode(lobby, 43)
	assert.NotEqual(t, sameSeedA, nextSeed)
}

func TestDefaultProvider_GenerateInviteCode_DifferentLobbies(t *testing.T) {
	t.Parallel()

	provider := DefaultProvider{}

	lobby1 := &component.LobbyComponent{ID: "lobby-1"}
	lobby2 := &component.LobbyComponent{ID: "lobby-2"}

	// Same seed: the lobby ID alone must be enough to separate them.
	code1 := provider.GenerateInviteCode(lobby1, 1)
	code2 := provider.GenerateInviteCode(lobby2, 1)

	assert.NotEqual(t, code1, code2)
}

func TestInviteCodeCharset(t *testing.T) {
	t.Parallel()

	// Verify charset excludes confusing characters
	assert.NotContains(t, inviteCodeCharset, "0")
	assert.NotContains(t, inviteCodeCharset, "O")
	assert.NotContains(t, inviteCodeCharset, "I")
	assert.NotContains(t, inviteCodeCharset, "L")
	assert.NotContains(t, inviteCodeCharset, "1")

	// Verify charset contains expected characters
	assert.Contains(t, inviteCodeCharset, "A")
	assert.Contains(t, inviteCodeCharset, "Z")
	assert.Contains(t, inviteCodeCharset, "2")
	assert.Contains(t, inviteCodeCharset, "9")
}

func TestTeamConfig(t *testing.T) {
	t.Parallel()

	config := TeamConfig{
		TeamID:     "alpha",
		MaxPlayers: 5,
	}

	assert.Equal(t, "alpha", config.TeamID)
	assert.Equal(t, 5, config.MaxPlayers)
}

func TestCommandNames(t *testing.T) {
	t.Parallel()

	// Verify command names are correct
	assert.Equal(t, "lobby_create", CreateLobbyCommand{}.Name())
	assert.Equal(t, "lobby_join", JoinLobbyCommand{}.Name())
	assert.Equal(t, "lobby_join_team", JoinTeamCommand{}.Name())
	assert.Equal(t, "lobby_leave", LeaveLobbyCommand{}.Name())
	assert.Equal(t, "lobby_set_ready", SetReadyCommand{}.Name())
	assert.Equal(t, "lobby_kick", KickPlayerCommand{}.Name())
	assert.Equal(t, "lobby_transfer_leader", TransferLeaderCommand{}.Name())
	assert.Equal(t, "lobby_start_session", StartSessionCommand{}.Name())
	assert.Equal(t, "lobby_generate_invite", GenerateInviteCodeCommand{}.Name())
	assert.Equal(t, "lobby_get_player", GetPlayerCommand{}.Name())
	assert.Equal(t, "lobby_get_all_players", GetAllPlayersCommand{}.Name())
}

func TestCrossShardCommandNames(t *testing.T) {
	t.Parallel()

	// Verify cross-shard command names are correct
	assert.Equal(t, "lobby_notify_session_start", NotifySessionStartCommand{}.Name())
	assert.Equal(t, "lobby_notify_session_end", NotifySessionEndCommand{}.Name())
	assert.Equal(t, "lobby_assign_shard", AssignShardCommand{}.Name())
}

func TestAssignShardCommand_Shape(t *testing.T) {
	t.Parallel()

	// Fields the handler reads — keep signatures stable so orchestrators
	// can't break silently if a field is renamed.
	cmd := AssignShardCommand{
		LobbyID:   "lobby-1",
		RequestID: "req-42",
		GameWorld: component.ShardAddress{ShardID: "game-shard-3"},
	}
	assert.Equal(t, "lobby-1", cmd.LobbyID)
	assert.Equal(t, "req-42", cmd.RequestID)
	assert.Equal(t, "game-shard-3", cmd.GameWorld.ShardID)
	assert.Empty(t, cmd.Reason)

	// Failure-path shape: empty GameWorld.ShardID + Reason triggers
	// the handler's failure branch without hitting cross-shard dispatch.
	fail := AssignShardCommand{
		Reason: "pool full",
	}
	assert.Empty(t, fail.GameWorld.ShardID)
	assert.Equal(t, "pool full", fail.Reason)
}

func TestEventNames(t *testing.T) {
	t.Parallel()

	// Verify event names are correct
	assert.Equal(t, "lobby_created", LobbyCreatedEvent{}.Name())
	assert.Equal(t, "lobby_player_joined", PlayerJoinedEvent{}.Name())
	assert.Equal(t, "lobby_player_left", PlayerLeftEvent{}.Name())
	assert.Equal(t, "lobby_player_kicked", PlayerKickedEvent{}.Name())
	assert.Equal(t, "lobby_player_ready", PlayerReadyEvent{}.Name())
	assert.Equal(t, "lobby_player_changed_team", PlayerChangedTeamEvent{}.Name())
	assert.Equal(t, "lobby_leader_changed", LeaderChangedEvent{}.Name())
	assert.Equal(t, "lobby_session_started", SessionStartedEvent{}.Name())
	assert.Equal(t, "lobby_session_ended", SessionEndedEvent{}.Name())
	assert.Equal(t, "lobby_session_awaiting_allocation", SessionAwaitingAllocationEvent{}.Name())
	assert.Equal(t, "lobby_invite_generated", InviteCodeGeneratedEvent{}.Name())
	assert.Equal(t, "lobby_deleted", LobbyDeletedEvent{}.Name())
	assert.Equal(t, "lobby_session_passthrough_updated", SessionPassthroughUpdatedEvent{}.Name())
	assert.Equal(t, "lobby_player_passthrough_updated", PlayerPassthroughUpdatedEvent{}.Name())
}

func TestCommandResultNames(t *testing.T) {
	t.Parallel()

	// CommandResult names are request-prefixed for targeted delivery
	requestID := "req-123"
	tests := []struct {
		name     string
		result   interface{ Name() string }
		expected string
	}{
		{
			name:     "CreateLobbyResult",
			result:   CreateLobbyResult{RequestID: requestID},
			expected: "req-123_create_lobby_result",
		},
		{
			name:     "JoinLobbyResult",
			result:   JoinLobbyResult{RequestID: requestID},
			expected: "req-123_join_lobby_result",
		},
		{
			name:     "JoinTeamResult",
			result:   JoinTeamResult{RequestID: requestID},
			expected: "req-123_join_team_result",
		},
		{
			name:     "LeaveLobbyResult",
			result:   LeaveLobbyResult{RequestID: requestID},
			expected: "req-123_leave_lobby_result",
		},
		{
			name:     "SetReadyResult",
			result:   SetReadyResult{RequestID: requestID},
			expected: "req-123_set_ready_result",
		},
		{
			name:     "KickPlayerResult",
			result:   KickPlayerResult{RequestID: requestID},
			expected: "req-123_kick_player_result",
		},
		{
			name:     "TransferLeaderResult",
			result:   TransferLeaderResult{RequestID: requestID},
			expected: "req-123_transfer_leader_result",
		},
		{
			name:     "StartSessionResult",
			result:   StartSessionResult{RequestID: requestID},
			expected: "req-123_start_session_result",
		},
		{
			name:     "GenerateInviteCodeResult",
			result:   GenerateInviteCodeResult{RequestID: requestID},
			expected: "req-123_generate_invite_code_result",
		},
		{
			name:     "GetPlayerResult",
			result:   GetPlayerResult{RequestID: requestID},
			expected: "req-123_get_player_result",
		},
		{
			name:     "GetAllPlayersResult",
			result:   GetAllPlayersResult{RequestID: requestID},
			expected: "req-123_get_all_players_result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.result.Name())
		})
	}
}

func TestCommandResultNames_DifferentRequestIDs(t *testing.T) {
	t.Parallel()

	// Verify different request IDs get different event names
	result1 := CreateLobbyResult{RequestID: "req-abc"}
	result2 := CreateLobbyResult{RequestID: "req-xyz"}

	assert.Equal(t, "req-abc_create_lobby_result", result1.Name())
	assert.Equal(t, "req-xyz_create_lobby_result", result2.Name())
	assert.NotEqual(t, result1.Name(), result2.Name())
}

func TestCommandResultFields(t *testing.T) {
	t.Parallel()

	// Test CreateLobbyResult with all fields
	createResult := CreateLobbyResult{
		RequestID: "req-123",
		IsSuccess: true,
		Message:   "lobby created",
		Lobby: component.LobbyComponent{
			ID:         "lobby-1",
			LeaderID:   "player1",
			InviteCode: "ABC123",
		},
	}
	assert.Equal(t, "req-123", createResult.RequestID)
	assert.True(t, createResult.IsSuccess)
	assert.Equal(t, "lobby created", createResult.Message)
	assert.Equal(t, "lobby-1", createResult.Lobby.ID)

	// Test GenerateInviteCodeResult with InviteCode field
	inviteResult := GenerateInviteCodeResult{
		InviteCode: "XYZ789",
	}
	assert.Equal(t, "XYZ789", inviteResult.InviteCode)
}

func TestCommandResultFailure(t *testing.T) {
	t.Parallel()

	// Test failure result
	result := CreateLobbyResult{
		IsSuccess: false,
		Message:   "player already in a lobby",
	}

	assert.False(t, result.IsSuccess)
	assert.Equal(t, "player already in a lobby", result.Message)
	assert.Empty(t, result.Lobby.ID) // Lobby should be empty on failure
}

func TestNotifySessionEndCommand(t *testing.T) {
	t.Parallel()

	cmd := NotifySessionEndCommand{
		LobbyID: "lobby-123",
	}

	assert.Equal(t, "lobby-123", cmd.LobbyID)
	assert.Equal(t, "lobby_notify_session_end", cmd.Name())
}

func TestGameWorld(t *testing.T) {
	t.Parallel()

	gameWorld := cardinal.OtherWorld{
		Region:       "us-west",
		Organization: "argus-labs",
		Project:      "my-game",
		ShardID:      "game-shard-1",
	}

	assert.Equal(t, "us-west", gameWorld.Region)
	assert.Equal(t, "argus-labs", gameWorld.Organization)
	assert.Equal(t, "my-game", gameWorld.Project)
	assert.Equal(t, "game-shard-1", gameWorld.ShardID)
}

func TestLobbyComponent_WithGameWorld(t *testing.T) {
	t.Parallel()

	lobby := component.LobbyComponent{
		GameWorld: component.ShardAddress{
			Region:  "eu-central",
			ShardID: "game-eu-1",
		},
		Session: component.Session{
			State: component.SessionStateIdle,
		},
	}

	assert.Equal(t, "game-eu-1", lobby.GameWorld.ShardID)
	assert.Equal(t, "eu-central", lobby.GameWorld.Region)
	assert.Equal(t, component.SessionStateIdle, lobby.Session.State)
}

func TestGetPlayerCommand(t *testing.T) {
	t.Parallel()

	// Test GetPlayerCommand with target player
	cmd := GetPlayerCommand{
		RequestID: "req-123",
		PlayerID:  "player-456",
	}

	assert.Equal(t, "req-123", cmd.RequestID)
	assert.Equal(t, "player-456", cmd.PlayerID)
	assert.Equal(t, "lobby_get_player", cmd.Name())

	// Test GetPlayerCommand with empty PlayerID (self)
	cmdSelf := GetPlayerCommand{
		PlayerID: "",
	}
	assert.Empty(t, cmdSelf.PlayerID)
}

func TestGetAllPlayersCommand(t *testing.T) {
	t.Parallel()

	cmd := GetAllPlayersCommand{
		RequestID: "req-123",
	}

	assert.Equal(t, "req-123", cmd.RequestID)
	assert.Equal(t, "lobby_get_all_players", cmd.Name())
}

func TestGetPlayerResult(t *testing.T) {
	t.Parallel()

	// Test success case
	result := GetPlayerResult{
		RequestID: "req-123",
		IsSuccess: true,
		Message:   "player found",
		Player: component.PlayerComponent{
			PlayerID:        "player-456",
			LobbyID:         "lobby-789",
			TeamID:          "team-1",
			IsReady:         true,
			PassthroughData: `{"level":10}`,
			JoinedAt:        1234567890,
		},
	}

	assert.Equal(t, "req-123", result.RequestID)
	assert.True(t, result.IsSuccess)
	assert.Equal(t, "player found", result.Message)
	assert.Equal(t, "player-456", result.Player.PlayerID)
	assert.Equal(t, "lobby-789", result.Player.LobbyID)
	assert.Equal(t, "team-1", result.Player.TeamID)
	assert.True(t, result.Player.IsReady)
	assert.JSONEq(t, `{"level":10}`, result.Player.PassthroughData)

	// Test failure case
	failResult := GetPlayerResult{
		IsSuccess: false,
	}
	assert.False(t, failResult.IsSuccess)
	assert.Empty(t, failResult.Player.PlayerID)
}

func TestGetAllPlayersResult(t *testing.T) {
	t.Parallel()

	// Test success case with multiple players
	result := GetAllPlayersResult{
		RequestID: "req-123",
		IsSuccess: true,
		Message:   "players found",
		Players: [component.MaxLobbyPlayers]component.PlayerComponent{
			{
				PlayerID: "player-1",
				LobbyID:  "lobby-1",
				TeamID:   "team-1",
				IsReady:  true,
			},
			{
				PlayerID: "player-2",
				LobbyID:  "lobby-1",
				TeamID:   "team-2",
				IsReady:  false,
			},
		},
		PlayersCount: 2,
	}

	assert.Equal(t, "req-123", result.RequestID)
	assert.True(t, result.IsSuccess)
	assert.Equal(t, "players found", result.Message)
	assert.Equal(t, 2, result.PlayersCount)
	assert.Equal(t, "player-1", result.Players[0].PlayerID)
	assert.Equal(t, "player-2", result.Players[1].PlayerID)

	// Test failure case
	failResult := GetAllPlayersResult{
		IsSuccess: false,
	}
	assert.False(t, failResult.IsSuccess)
	assert.Zero(t, failResult.PlayersCount)
}

func TestResultsWithPlayerComponent(t *testing.T) {
	t.Parallel()

	player := component.PlayerComponent{
		PlayerID:        "player-123",
		LobbyID:         "lobby-456",
		TeamID:          "team-1",
		IsReady:         true,
		PassthroughData: `{"skin":"blue"}`,
		JoinedAt:        1234567890,
	}

	// Test CreateLobbyResult includes Player
	createResult := CreateLobbyResult{
		Player: player,
	}
	assert.Equal(t, "player-123", createResult.Player.PlayerID)
	assert.Equal(t, "lobby-456", createResult.Player.LobbyID)

	// Test JoinLobbyResult includes PlayersList
	joinResult := JoinLobbyResult{
		PlayersList: [component.MaxLobbyPlayers]component.PlayerComponent{
			player,
			{PlayerID: "player-other"},
		},
		PlayersListCount: 2,
	}
	assert.Equal(t, 2, joinResult.PlayersListCount)
	assert.Equal(t, "player-123", joinResult.PlayersList[0].PlayerID)
	assert.Equal(t, "player-other", joinResult.PlayersList[1].PlayerID)

	// Test JoinTeamResult includes Player
	joinTeamResult := JoinTeamResult{
		Player: player,
	}
	assert.Equal(t, "player-123", joinTeamResult.Player.PlayerID)
	assert.Equal(t, "team-1", joinTeamResult.Player.TeamID)

	// Test SetReadyResult includes Player
	setReadyResult := SetReadyResult{
		Player: player,
	}
	assert.Equal(t, "player-123", setReadyResult.Player.PlayerID)
	assert.True(t, setReadyResult.Player.IsReady)

	// Test UpdatePlayerPassthroughResult includes Player
	updateResult := UpdatePlayerPassthroughResult{
		Player: player,
	}
	assert.Equal(t, "player-123", updateResult.Player.PlayerID)
	assert.JSONEq(t, `{"skin":"blue"}`, updateResult.Player.PassthroughData)
}

func TestEventsWithPlayerComponent(t *testing.T) {
	t.Parallel()

	player := component.PlayerComponent{
		PlayerID:        "player-123",
		LobbyID:         "lobby-456",
		TeamID:          "team-1",
		IsReady:         true,
		PassthroughData: `{"level":5}`,
		JoinedAt:        1234567890,
	}

	// Test PlayerJoinedEvent includes Player
	joinedEvent := PlayerJoinedEvent{
		TeamID: "alpha",
		Player: player,
	}
	assert.Equal(t, "player-123", joinedEvent.Player.PlayerID)
	assert.Equal(t, "alpha", joinedEvent.TeamID)

	// Test PlayerReadyEvent includes Player
	readyEvent := PlayerReadyEvent{
		Player: player,
	}
	assert.Equal(t, "player-123", readyEvent.Player.PlayerID)
	assert.True(t, readyEvent.Player.IsReady)

	// Test PlayerChangedTeamEvent includes Player
	changedTeamEvent := PlayerChangedTeamEvent{
		OldTeamID: "alpha",
		NewTeamID: "beta",
		Player:    player,
	}
	assert.Equal(t, "player-123", changedTeamEvent.Player.PlayerID)
	assert.Equal(t, "alpha", changedTeamEvent.OldTeamID)
	assert.Equal(t, "beta", changedTeamEvent.NewTeamID)

	// Test PlayerPassthroughUpdatedEvent includes Player
	passthroughEvent := PlayerPassthroughUpdatedEvent{
		Player: player,
	}
	assert.Equal(t, "player-123", passthroughEvent.Player.PlayerID)
	assert.JSONEq(t, `{"level":5}`, passthroughEvent.Player.PassthroughData)
}

func TestFindTargetTeam(t *testing.T) {
	t.Parallel()

	lobby := lobbyWithTeams(t,
		[]component.Team{
			{TeamID: "alpha", MaxPlayers: 2}, // filled below
			{TeamID: "beta", MaxPlayers: 2},  // has space
			{TeamID: "gamma", MaxPlayers: 0}, // unlimited
		},
		map[string][]string{"alpha": {"p1", "p2"}, "beta": {"p3"}},
	)

	tests := []struct {
		name       string
		teamID     string
		wantTeamID string
		wantErrMsg string
	}{
		{
			name:       "find by id - exists with space",
			teamID:     "beta",
			wantTeamID: "beta",
			wantErrMsg: "",
		},
		{
			name:       "find by id - team not found",
			teamID:     "nonexistent",
			wantTeamID: "",
			wantErrMsg: "team not found",
		},
		{
			name:       "find by id - team is full",
			teamID:     "alpha",
			wantTeamID: "",
			wantErrMsg: "team is full",
		},
		{
			name:       "auto-assign - finds first with space",
			teamID:     "",
			wantTeamID: "beta",
			wantErrMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			team, errMsg := findTargetTeam(lobby, tt.teamID)
			if tt.wantErrMsg != "" {
				assert.Nil(t, team)
				assert.Equal(t, tt.wantErrMsg, errMsg)
			} else {
				assert.NotNil(t, team)
				assert.Equal(t, tt.wantTeamID, team.TeamID)
				assert.Empty(t, errMsg)
			}
		})
	}
}

func TestFindTargetTeam_AllTeamsFull(t *testing.T) {
	t.Parallel()

	lobby := lobbyWithTeams(t,
		[]component.Team{
			{TeamID: "alpha", MaxPlayers: 1},
			{TeamID: "beta", MaxPlayers: 1},
		},
		map[string][]string{"alpha": {"p1"}, "beta": {"p2"}},
	)

	team, errMsg := findTargetTeam(lobby, "")
	assert.Nil(t, team)
	assert.Equal(t, "all teams are full", errMsg)
}

func lobbyWithTeams(
	t *testing.T,
	teams []component.Team,
	membership map[string][]string,
) *component.LobbyComponent {
	t.Helper()
	lobby := &component.LobbyComponent{}
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

// A preset is server-owned config, so validatePreset guards against a deployment promising seats the
// component's fixed roster cannot physically hold.
func TestValidatePreset(t *testing.T) {
	t.Parallel()

	tooManyTeams := make([]component.TeamConfig, component.MaxLobbyTeams+1)
	for i := range tooManyTeams {
		tooManyTeams[i] = component.TeamConfig{TeamID: "team" + strconv.Itoa(i), MaxPlayers: 1}
	}

	tests := []struct {
		name  string
		teams []component.TeamConfig
		want  string
	}{
		{
			name:  "no teams",
			teams: nil,
			want:  "no teams",
		},
		{
			name:  "more teams than a lobby holds",
			teams: tooManyTeams,
			want:  "declares 5 teams, more than the 4 a lobby can hold",
		},
		{
			name: "duplicate team id",
			teams: []component.TeamConfig{
				{TeamID: "red", MaxPlayers: 2},
				{TeamID: "red", MaxPlayers: 2},
			},
			want: "duplicate team id red",
		},
		{
			name:  "single team over the roster cap",
			teams: []component.TeamConfig{{TeamID: "red", MaxPlayers: component.MaxLobbyPlayers + 1}},
			want:  `team "red" allows 17 players, more than the 16 a lobby can hold`,
		},
		{
			name: "teams fit individually but not together",
			teams: []component.TeamConfig{
				{TeamID: "red", MaxPlayers: 10},
				{TeamID: "blue", MaxPlayers: 10},
			},
			want: "teams allow 20 players in total, more than the 16 a lobby can hold",
		},
		{
			name: "exactly at the roster cap",
			teams: []component.TeamConfig{
				{TeamID: "red", MaxPlayers: 8},
				{TeamID: "blue", MaxPlayers: 8},
			},
			want: "",
		},
		// An unlimited team makes the total meaningless — the roster cap is what bounds it, and
		// AddPlayerToTeam enforces that at runtime.
		{
			name: "unlimited team alongside a bounded one",
			teams: []component.TeamConfig{
				{TeamID: "red", MaxPlayers: 0},
				{TeamID: "blue", MaxPlayers: 12},
			},
			want: "",
		},
		{
			name:  "rampage coop_4p",
			teams: []component.TeamConfig{{TeamID: "default", MaxPlayers: 4}},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, validatePreset(tt.teams))
		})
	}
}

func TestResolvePresetRejectsMisconfigured(t *testing.T) {
	t.Parallel()

	presets := map[string][]component.TeamConfig{
		"good": {{TeamID: "default", MaxPlayers: 4}},
		"bad":  {{TeamID: "red", MaxPlayers: 99}},
	}

	teams, errMsg := resolvePreset("good", presets)
	require.Empty(t, errMsg)
	assert.Len(t, teams, 1)

	_, errMsg = resolvePreset("bad", presets)
	assert.Equal(t, `preset misconfigured: team "red" allows 99 players, more than the 16 a lobby can hold`, errMsg)

	_, errMsg = resolvePreset("", presets)
	assert.Equal(t, "preset is required", errMsg)

	_, errMsg = resolvePreset("missing", presets)
	assert.Equal(t, "unknown preset: missing", errMsg)
}

// SetConfig panics rather than returning an error because cardinal.RegisterPlugin has no error path:
// a bad preset has to stop the boot, or it silently rejects every CreateLobbyCommand instead.
func TestSetConfigPanicsOnUnusablePreset(t *testing.T) {
	assert.PanicsWithValue(t,
		`lobby preset "broken" is unusable: teams allow 32 players in total, more than the 16 a lobby can hold`,
		func() {
			SetConfig(Config{}, map[string][]component.TeamConfig{
				"broken": {{TeamID: "red", MaxPlayers: 16}, {TeamID: "blue", MaxPlayers: 16}},
			})
		})
}

func TestConfig_AssignmentFields(t *testing.T) {
	t.Parallel()

	// Assignment-related config fields. AssignmentAuthority is an
	// accident-prevention filter (not authentication — cmd.Persona is
	// not verified at this layer). MaxAllocationTimeout bounds the
	// pending-allocation lifetime.
	cfg := Config{
		AssignmentAuthority:  "region.world.org.project.lobby",
		MaxAllocationTimeout: 300,
	}
	assert.Equal(t, "region.world.org.project.lobby", cfg.AssignmentAuthority)
	assert.Equal(t, int64(300), cfg.MaxAllocationTimeout)

	// MaxAllocationTimeout <= 0 is the documented "disabled" sentinel.
	disabled := Config{MaxAllocationTimeout: 0}
	assert.LessOrEqual(t, disabled.MaxAllocationTimeout, int64(0))
	negDisabled := Config{MaxAllocationTimeout: -1}
	assert.LessOrEqual(t, negDisabled.MaxAllocationTimeout, int64(0))
}
