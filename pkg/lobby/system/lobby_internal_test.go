package system

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/lobby/component"
	"github.com/stretchr/testify/assert"
)

func TestDefaultProvider_GenerateInviteCode(t *testing.T) {
	t.Parallel()

	provider := DefaultProvider{}
	lobby := &component.LobbyComponent{
		ID: "test-lobby-id",
	}

	// Generate multiple codes
	codes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code := provider.GenerateInviteCode(lobby)

		// Check length
		assert.Len(t, code, 6)

		// Check charset (should only contain valid characters)
		for _, c := range code {
			assert.Contains(t, inviteCodeCharset, string(c), "invalid character in code: %c", c)
		}

		// Store for uniqueness check (note: may have some collisions due to timing)
		codes[code] = true
	}

	// Should have generated mostly unique codes
	// (allowing some collisions due to fast iteration)
	assert.GreaterOrEqual(t, len(codes), 50, "too many duplicate codes generated")
}

func TestDefaultProvider_GenerateInviteCode_DifferentLobbies(t *testing.T) {
	t.Parallel()

	provider := DefaultProvider{}

	lobby1 := &component.LobbyComponent{ID: "lobby-1"}
	lobby2 := &component.LobbyComponent{ID: "lobby-2"}

	code1 := provider.GenerateInviteCode(lobby1)
	code2 := provider.GenerateInviteCode(lobby2)

	// Different lobbies should generate different codes
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
		Name:       "Team Alpha",
		MaxPlayers: 5,
	}

	assert.Equal(t, "Team Alpha", config.Name)
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

func TestNotifySessionStartCommand(t *testing.T) {
	t.Parallel()

	// Test NotifySessionStartCommand contains all required fields
	cmd := NotifySessionStartCommand{
		Lobby: component.LobbyComponent{
			ID:         "lobby-123",
			LeaderID:   "player1",
			InviteCode: "ABC123",
			GameWorld: cardinal.OtherWorld{
				Region:       "us-west",
				Organization: "myorg",
				Project:      "myproject",
				ShardID:      "game-shard-1",
			},
			Teams: []component.Team{
				{
					TeamID:    "team1",
					Name:      "Team Alpha",
					PlayerIDs: []string{"player1", "player2"},
				},
			},
		},
		// LobbyWorld would be cardinal.OtherWorld in real usage
	}

	assert.Equal(t, "lobby-123", cmd.Lobby.ID)
	assert.Equal(t, "player1", cmd.Lobby.LeaderID)
	assert.Equal(t, "game-shard-1", cmd.Lobby.GameWorld.ShardID)
	assert.Len(t, cmd.Lobby.Teams, 1)
	assert.Equal(t, 2, cmd.Lobby.PlayerCount())
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

func TestCreateLobbyCommand_WithGameWorld(t *testing.T) {
	t.Parallel()

	cmd := CreateLobbyCommand{
		RequestID: "req-123",
		Teams: []TeamConfig{
			{Name: "Team 1", MaxPlayers: 4},
			{Name: "Team 2", MaxPlayers: 4},
		},
		GameWorld: cardinal.OtherWorld{
			Region:       "us-west",
			Organization: "myorg",
			Project:      "myproject",
			ShardID:      "game-shard-1",
		},
	}

	assert.Equal(t, "req-123", cmd.RequestID)
	assert.Len(t, cmd.Teams, 2)
	assert.Equal(t, "game-shard-1", cmd.GameWorld.ShardID)
	assert.Equal(t, "lobby_create", cmd.Name())
}

func TestLobbyComponent_WithGameWorld(t *testing.T) {
	t.Parallel()

	lobby := component.LobbyComponent{
		GameWorld: cardinal.OtherWorld{
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

func TestStartSessionPayloadAlias(t *testing.T) {
	t.Parallel()

	// StartSessionPayload is an alias for NotifySessionStartCommand
	var payload StartSessionPayload
	payload.Lobby = component.LobbyComponent{ID: "lobby-1"}

	// Should be assignable to NotifySessionStartCommand
	var cmd = payload
	assert.Equal(t, "lobby-1", cmd.Lobby.ID)
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
			PassthroughData: map[string]any{"level": 10},
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
	assert.Equal(t, 10, result.Player.PassthroughData["level"])

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
		Players: []component.PlayerComponent{
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
	}

	assert.Equal(t, "req-123", result.RequestID)
	assert.True(t, result.IsSuccess)
	assert.Equal(t, "players found", result.Message)
	assert.Len(t, result.Players, 2)
	assert.Equal(t, "player-1", result.Players[0].PlayerID)
	assert.Equal(t, "player-2", result.Players[1].PlayerID)

	// Test failure case
	failResult := GetAllPlayersResult{
		IsSuccess: false,
	}
	assert.False(t, failResult.IsSuccess)
	assert.Nil(t, failResult.Players)
}

func TestResultsWithPlayerComponent(t *testing.T) {
	t.Parallel()

	player := component.PlayerComponent{
		PlayerID:        "player-123",
		LobbyID:         "lobby-456",
		TeamID:          "team-1",
		IsReady:         true,
		PassthroughData: map[string]any{"skin": "blue"},
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
		PlayersList: []component.PlayerComponent{
			player,
			{PlayerID: "player-other"},
		},
	}
	assert.Len(t, joinResult.PlayersList, 2)
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
	assert.Equal(t, "blue", updateResult.Player.PassthroughData["skin"])
}

func TestEventsWithPlayerComponent(t *testing.T) {
	t.Parallel()

	player := component.PlayerComponent{
		PlayerID:        "player-123",
		LobbyID:         "lobby-456",
		TeamID:          "team-1",
		IsReady:         true,
		PassthroughData: map[string]any{"level": 5},
		JoinedAt:        1234567890,
	}

	// Test PlayerJoinedEvent includes Player
	joinedEvent := PlayerJoinedEvent{
		TeamName: "Team Alpha",
		Player:   player,
	}
	assert.Equal(t, "player-123", joinedEvent.Player.PlayerID)
	assert.Equal(t, "Team Alpha", joinedEvent.TeamName)

	// Test PlayerReadyEvent includes Player
	readyEvent := PlayerReadyEvent{
		Player: player,
	}
	assert.Equal(t, "player-123", readyEvent.Player.PlayerID)
	assert.True(t, readyEvent.Player.IsReady)

	// Test PlayerChangedTeamEvent includes Player
	changedTeamEvent := PlayerChangedTeamEvent{
		OldTeamName: "Team Alpha",
		NewTeamName: "Team Beta",
		Player:      player,
	}
	assert.Equal(t, "player-123", changedTeamEvent.Player.PlayerID)
	assert.Equal(t, "Team Alpha", changedTeamEvent.OldTeamName)
	assert.Equal(t, "Team Beta", changedTeamEvent.NewTeamName)

	// Test PlayerPassthroughUpdatedEvent includes Player
	passthroughEvent := PlayerPassthroughUpdatedEvent{
		Player: player,
	}
	assert.Equal(t, "player-123", passthroughEvent.Player.PlayerID)
	assert.Equal(t, 5, passthroughEvent.Player.PassthroughData["level"])
}

func TestFindTargetTeam(t *testing.T) {
	t.Parallel()

	lobby := &component.LobbyComponent{
		Teams: []component.Team{
			{TeamID: "team1", Name: "Alpha", MaxPlayers: 2, PlayerIDs: []string{"p1", "p2"}}, // full
			{TeamID: "team2", Name: "Beta", MaxPlayers: 2, PlayerIDs: []string{"p3"}},        // has space
			{TeamID: "team3", Name: "Gamma", MaxPlayers: 0, PlayerIDs: []string{}},           // unlimited
		},
	}

	tests := []struct {
		name       string
		teamName   string
		wantTeamID string
		wantErrMsg string
	}{
		{
			name:       "find by name - exists with space",
			teamName:   "Beta",
			wantTeamID: "team2",
			wantErrMsg: "",
		},
		{
			name:       "find by name - team not found",
			teamName:   "NonExistent",
			wantTeamID: "",
			wantErrMsg: "team not found",
		},
		{
			name:       "find by name - team is full",
			teamName:   "Alpha",
			wantTeamID: "",
			wantErrMsg: "team is full",
		},
		{
			name:       "auto-assign - finds first with space",
			teamName:   "",
			wantTeamID: "team2",
			wantErrMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			team, errMsg := findTargetTeam(lobby, tt.teamName)
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

	lobby := &component.LobbyComponent{
		Teams: []component.Team{
			{TeamID: "team1", Name: "Alpha", MaxPlayers: 1, PlayerIDs: []string{"p1"}},
			{TeamID: "team2", Name: "Beta", MaxPlayers: 1, PlayerIDs: []string{"p2"}},
		},
	}

	team, errMsg := findTargetTeam(lobby, "")
	assert.Nil(t, team)
	assert.Equal(t, "all teams are full", errMsg)
}
