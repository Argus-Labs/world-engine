package system

import (
	"testing"

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
	assert.Equal(t, "lobby_end_session", EndSessionCommand{}.Name())
	assert.Equal(t, "lobby_generate_invite", GenerateInviteCodeCommand{}.Name())
	assert.Equal(t, "lobby_notify_session_start", NotifySessionStartCommand{}.Name())
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
	assert.Equal(t, "lobby_error", LobbyErrorEvent{}.Name())
	assert.Equal(t, "lobby_deleted", LobbyDeletedEvent{}.Name())
}
