package types

import (
	"testing"
	"time"

	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

func TestLobby_HasTeams(t *testing.T) {
	lobby := &Lobby{}
	if lobby.HasTeams() {
		t.Error("expected lobby not to have teams")
	}

	lobby.Teams = []LobbyTeam{
		{Name: "Red", PartyIDs: []string{"party-1"}},
	}
	if !lobby.HasTeams() {
		t.Error("expected lobby to have teams")
	}
}

func TestLobby_PartyCount(t *testing.T) {
	lobby := &Lobby{}
	if lobby.PartyCount() != 0 {
		t.Errorf("expected party count 0, got %d", lobby.PartyCount())
	}

	lobby.Parties = []string{"party-1", "party-2", "party-3"}
	if lobby.PartyCount() != 3 {
		t.Errorf("expected party count 3, got %d", lobby.PartyCount())
	}
}

func TestLobby_HasParty(t *testing.T) {
	lobby := &Lobby{
		Parties: []string{"party-1", "party-2"},
	}

	if !lobby.HasParty("party-1") {
		t.Error("expected party-1 to be in lobby")
	}
	if !lobby.HasParty("party-2") {
		t.Error("expected party-2 to be in lobby")
	}
	if lobby.HasParty("party-3") {
		t.Error("expected party-3 not to be in lobby")
	}
}

func TestLobby_IsHost(t *testing.T) {
	lobby := &Lobby{
		HostPartyID: "party-1",
		Parties:     []string{"party-1", "party-2"},
	}

	if !lobby.IsHost("party-1") {
		t.Error("expected party-1 to be host")
	}
	if lobby.IsHost("party-2") {
		t.Error("expected party-2 not to be host")
	}
}

func TestLobby_GetTeamForParty(t *testing.T) {
	lobby := &Lobby{
		Teams: []LobbyTeam{
			{Name: "Red", PartyIDs: []string{"party-1", "party-2"}},
			{Name: "Blue", PartyIDs: []string{"party-3", "party-4"}},
		},
	}

	if team := lobby.GetTeamForParty("party-1"); team != "Red" {
		t.Errorf("expected team Red, got %s", team)
	}
	if team := lobby.GetTeamForParty("party-2"); team != "Red" {
		t.Errorf("expected team Red, got %s", team)
	}
	if team := lobby.GetTeamForParty("party-3"); team != "Blue" {
		t.Errorf("expected team Blue, got %s", team)
	}
	if team := lobby.GetTeamForParty("party-5"); team != "" {
		t.Errorf("expected empty team, got %s", team)
	}
}

func TestLobby_CanJoin(t *testing.T) {
	tests := []struct {
		state   LobbyState
		canJoin bool
	}{
		{LobbyStateWaiting, true},
		{LobbyStateReady, false},
		{LobbyStateInGame, false},
		{LobbyStateEnded, false},
	}

	for _, tt := range tests {
		lobby := &Lobby{State: tt.state}
		if lobby.CanJoin() != tt.canJoin {
			t.Errorf("state %s: expected CanJoin %v, got %v", tt.state, tt.canJoin, lobby.CanJoin())
		}
	}
}

func TestLobby_CanStart(t *testing.T) {
	tests := []struct {
		state    LobbyState
		canStart bool
	}{
		{LobbyStateWaiting, false},
		{LobbyStateReady, true},
		{LobbyStateInGame, false},
		{LobbyStateEnded, false},
	}

	for _, tt := range tests {
		lobby := &Lobby{State: tt.state}
		if lobby.CanStart() != tt.canStart {
			t.Errorf("state %s: expected CanStart %v, got %v", tt.state, tt.canStart, lobby.CanStart())
		}
	}
}

func TestLobby_CanLeave(t *testing.T) {
	tests := []struct {
		state    LobbyState
		canLeave bool
	}{
		{LobbyStateWaiting, true},
		{LobbyStateReady, true},
		{LobbyStateInGame, false},
		{LobbyStateEnded, false},
	}

	for _, tt := range tests {
		lobby := &Lobby{State: tt.state}
		if lobby.CanLeave() != tt.canLeave {
			t.Errorf("state %s: expected CanLeave %v, got %v", tt.state, tt.canLeave, lobby.CanLeave())
		}
	}
}

func TestLobbyState_Values(t *testing.T) {
	// Verify state string values
	if LobbyStateWaiting != "waiting" {
		t.Errorf("expected waiting, got %s", LobbyStateWaiting)
	}
	if LobbyStateReady != "ready" {
		t.Errorf("expected ready, got %s", LobbyStateReady)
	}
	if LobbyStateInGame != "in_game" {
		t.Errorf("expected in_game, got %s", LobbyStateInGame)
	}
	if LobbyStateEnded != "ended" {
		t.Errorf("expected ended, got %s", LobbyStateEnded)
	}
}

func TestLobby_FullStruct(t *testing.T) {
	now := time.Now()
	startedAt := now.Add(5 * time.Minute)
	heartbeat := now.Add(10 * time.Minute)

	lobby := &Lobby{
		MatchID:     "match-1",
		HostPartyID: "party-1",
		Parties:     []string{"party-1", "party-2"},
		Teams: []LobbyTeam{
			{Name: "Red", PartyIDs: []string{"party-1"}},
			{Name: "Blue", PartyIDs: []string{"party-2"}},
		},
		State:            LobbyStateInGame,
		MatchProfileName: "5v5",
		MatchmakingAddress: &microv1.ServiceAddress{
			Region:       "us-west",
			Realm:        microv1.ServiceAddress_REALM_WORLD,
			Organization: "test-org",
			Project:      "test-project",
			ServiceId:    "matchmaking-shard",
		},
		TargetAddress: &microv1.ServiceAddress{
			Region:       "us-west",
			Realm:        microv1.ServiceAddress_REALM_WORLD,
			Organization: "test-org",
			Project:      "test-project",
			ServiceId:    "game-shard",
		},
		Config:        map[string]any{"mode": "ranked"},
		MinPlayers:    2,
		MaxPlayers:    10,
		CreatedAt:     now,
		StartedAt:     &startedAt,
		LastHeartbeat: &heartbeat,
	}

	if lobby.MatchID != "match-1" {
		t.Errorf("expected MatchID match-1, got %s", lobby.MatchID)
	}
	if lobby.HostPartyID != "party-1" {
		t.Errorf("expected host party-1, got %s", lobby.HostPartyID)
	}
	if !lobby.HasTeams() {
		t.Error("expected lobby to have teams")
	}
	if lobby.PartyCount() != 2 {
		t.Errorf("expected 2 parties, got %d", lobby.PartyCount())
	}
	if lobby.MatchProfileName != "5v5" {
		t.Errorf("expected profile 5v5, got %s", lobby.MatchProfileName)
	}
	if lobby.MatchmakingAddress == nil {
		t.Error("expected matchmaking address to be set")
	}
	if lobby.TargetAddress == nil {
		t.Error("expected target address to be set")
	}
	if lobby.MinPlayers != 2 {
		t.Errorf("expected min players 2, got %d", lobby.MinPlayers)
	}
	if lobby.MaxPlayers != 10 {
		t.Errorf("expected max players 10, got %d", lobby.MaxPlayers)
	}
	if !lobby.CreatedAt.Equal(now) {
		t.Errorf("expected created at %v, got %v", now, lobby.CreatedAt)
	}
	if lobby.StartedAt == nil || !lobby.StartedAt.Equal(startedAt) {
		t.Error("expected started at to be set")
	}
	if lobby.LastHeartbeat == nil || !lobby.LastHeartbeat.Equal(heartbeat) {
		t.Error("expected last heartbeat to be set")
	}
}

func TestLobby_IsDisconnected(t *testing.T) {
	lobby := &Lobby{
		Parties:             []string{"party-1", "party-2", "party-3"},
		DisconnectedParties: []string{"party-2"},
	}

	if lobby.IsDisconnected("party-1") {
		t.Error("expected party-1 not to be disconnected")
	}
	if !lobby.IsDisconnected("party-2") {
		t.Error("expected party-2 to be disconnected")
	}
	if lobby.IsDisconnected("party-3") {
		t.Error("expected party-3 not to be disconnected")
	}
}

func TestLobbyTeam(t *testing.T) {
	team := LobbyTeam{
		Name:     "Red",
		PartyIDs: []string{"party-1", "party-2"},
	}

	if team.Name != "Red" {
		t.Errorf("expected name Red, got %s", team.Name)
	}
	if len(team.PartyIDs) != 2 {
		t.Errorf("expected 2 party IDs, got %d", len(team.PartyIDs))
	}
}
