package lobby_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/lobby"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (w *ownershipWorld) player(t *testing.T, playerID string) lobby.PlayerComponent {
	t.Helper()
	for entity := range w.world.Contains[struct{ Player lobby.PlayerComponent }]().Iter() {
		if p := entity.Get[lobby.PlayerComponent](); p.PlayerID == playerID {
			return p
		}
	}
	require.Failf(t, "player not found", "player_id=%s", playerID)
	return lobby.PlayerComponent{}
}

// forceLobbyState mutates the named lobby's component directly and persists it, bypassing the
// lobby systems. The ownership harness has no NATS connection, so the real in_session path
// (AssignShardCommand -> dispatchSessionStart -> SendToShard) panics during end-of-tick event
// dispatch; this reaches the same lobby state the handler reads without that send.
func (w *ownershipWorld) forceLobbyState(t *testing.T, lobbyID string, fn func(*lobby.Component)) {
	t.Helper()
	var ref cardinal.Entity
	var lob lobby.Component
	ok := false
	for entity := range w.world.Contains[struct{ Lobby lobby.Component }]().Iter() {
		if l := entity.Get[lobby.Component](); l.ID == lobbyID {
			ref, lob, ok = entity, l, true
			break
		}
	}
	require.True(t, ok, "lobby not found: %s", lobbyID)
	fn(&lob)
	ref.Set(lob)
}

// A player who learns a lobby ID from broadcast events must not be able to abort that lobby's
// pending session start by sending NotifySessionEndCommand (a Game -> Lobby shard command) over
// the client path. The lobby must stay in awaiting_allocation until the game shard ends it.
func TestNotifySessionEndCommand_PlayerSenderRejected_AwaitingAllocation(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	w := newOwnershipWorld(t, lobby.Config{
		LobbyPresets:     map[string][]lobby.TeamConfig{"solo": {{TeamID: "red", MaxPlayers: 4}}},
		HeartbeatTimeout: 3600,
	})

	w.send(t, "leader", lobby.CreateLobbyCommand{RequestID: "create", Preset: "solo"})
	w.tick(100)
	lobbyID := w.onlyLobby(t).ID

	w.send(t, "leader", lobby.SetReadyCommand{RequestID: "ready", IsReady: true})
	w.tick(101)

	w.send(t, "leader", lobby.StartSessionCommand{RequestID: "start"})
	w.tick(102)
	lob := w.onlyLobby(t)
	assert.Equal(t, lobby.SessionStateAwaitingAllocation, lob.Session.State)
	assert.Equal(t, "start", lob.Session.PendingRequestID)

	w.send(t, "mallory", lobby.NotifySessionEndCommand{LobbyID: lobbyID})
	w.tick(103)
	lob = w.onlyLobby(t)
	assert.Equal(t, lobby.SessionStateAwaitingAllocation, lob.Session.State,
		"player-sent NotifySessionEndCommand must be dropped")
	assert.Equal(t, "start", lob.Session.PendingRequestID,
		"pending request must stay intact when a player sender is dropped")

	w.sendFromShard(t, "project.game-shard-1", lobby.NotifySessionEndCommand{LobbyID: lobbyID})
	w.tick(104)
	lob = w.onlyLobby(t)
	assert.Equal(t, lobby.SessionStateIdle, lob.Session.State,
		"shard-sent NotifySessionEndCommand must abort the pending allocation")
	assert.Empty(t, lob.Session.PendingRequestID,
		"pending request must be cleared when a shard ends the session")
}

// A player who learns a lobby ID must not be able to tear down a live session. The lobby must stay
// in_session with ready flags intact until the game shard ends it; the shard sender must still work.
func TestNotifySessionEndCommand_PlayerSenderRejected_InSession(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	w := newOwnershipWorld(t, lobby.Config{
		LobbyPresets:     map[string][]lobby.TeamConfig{"solo": {{TeamID: "red", MaxPlayers: 4}}},
		HeartbeatTimeout: 3600,
	})

	w.send(t, "leader", lobby.CreateLobbyCommand{RequestID: "create", Preset: "solo"})
	w.tick(100)
	lobbyID := w.onlyLobby(t).ID

	w.send(t, "leader", lobby.SetReadyCommand{RequestID: "ready", IsReady: true})
	w.tick(101)
	assert.True(t, w.player(t, "leader").IsReady, "leader is ready before session starts")

	w.send(t, "leader", lobby.StartSessionCommand{RequestID: "start"})
	w.tick(102)
	assert.Equal(t, lobby.SessionStateAwaitingAllocation, w.onlyLobby(t).Session.State)

	w.forceLobbyState(t, lobbyID, func(lob *lobby.Component) {
		lob.Session.State = lobby.SessionStateInSession
		lob.Session.PendingRequestID = ""
		lob.Session.PendingStartedAt = 0
		lob.GameWorld = lobby.ShardAddress{Project: "project", ShardID: "game-shard-1"}
	})
	assert.Equal(t, lobby.SessionStateInSession, w.onlyLobby(t).Session.State)

	w.send(t, "mallory", lobby.NotifySessionEndCommand{LobbyID: lobbyID})
	w.tick(103)
	lob := w.onlyLobby(t)
	assert.Equal(t, lobby.SessionStateInSession, lob.Session.State,
		"player-sent NotifySessionEndCommand must be dropped")
	assert.True(t, w.player(t, "leader").IsReady,
		"ready flags must not reset when a player sender is dropped")

	w.sendFromShard(t, "project.game-shard-1", lobby.NotifySessionEndCommand{LobbyID: lobbyID})
	w.tick(104)
	lob = w.onlyLobby(t)
	assert.Equal(t, lobby.SessionStateIdle, lob.Session.State,
		"shard-sent NotifySessionEndCommand must end the session")
	assert.False(t, w.player(t, "leader").IsReady,
		"ready flags must reset when the session ends")
}
