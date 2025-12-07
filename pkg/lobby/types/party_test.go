package types

import (
	"testing"
	"time"
)

func TestParty_Size(t *testing.T) {
	party := &Party{
		Members: []string{"player-1", "player-2", "player-3"},
	}

	if party.Size() != 3 {
		t.Errorf("expected size 3, got %d", party.Size())
	}

	emptyParty := &Party{Members: []string{}}
	if emptyParty.Size() != 0 {
		t.Errorf("expected size 0, got %d", emptyParty.Size())
	}
}

func TestParty_IsFull(t *testing.T) {
	party := &Party{
		Members: []string{"player-1", "player-2"},
		MaxSize: 3,
	}

	if party.IsFull() {
		t.Error("expected party not to be full")
	}

	party.Members = append(party.Members, "player-3")
	if !party.IsFull() {
		t.Error("expected party to be full")
	}

	party.Members = append(party.Members, "player-4")
	if !party.IsFull() {
		t.Error("expected party to be full when over max")
	}
}

func TestParty_HasMember(t *testing.T) {
	party := &Party{
		Members: []string{"player-1", "player-2"},
	}

	if !party.HasMember("player-1") {
		t.Error("expected player-1 to be a member")
	}
	if !party.HasMember("player-2") {
		t.Error("expected player-2 to be a member")
	}
	if party.HasMember("player-3") {
		t.Error("expected player-3 not to be a member")
	}
}

func TestParty_IsLeader(t *testing.T) {
	party := &Party{
		LeaderID: "player-1",
		Members:  []string{"player-1", "player-2"},
	}

	if !party.IsLeader("player-1") {
		t.Error("expected player-1 to be leader")
	}
	if party.IsLeader("player-2") {
		t.Error("expected player-2 not to be leader")
	}
}

func TestParty_InLobby(t *testing.T) {
	party := &Party{
		ID: "party-1",
	}

	if party.InLobby() {
		t.Error("expected party not to be in lobby")
	}

	party.LobbyID = "lobby-1"
	if !party.InLobby() {
		t.Error("expected party to be in lobby")
	}
}

func TestParty_FullStruct(t *testing.T) {
	now := time.Now()
	party := &Party{
		ID:        "party-1",
		LeaderID:  "leader-1",
		Members:   []string{"leader-1", "player-2"},
		IsOpen:    true,
		MaxSize:   5,
		LobbyID:   "lobby-1",
		IsReady:   true,
		CreatedAt: now,
	}

	if party.ID != "party-1" {
		t.Errorf("expected ID party-1, got %s", party.ID)
	}
	if party.LeaderID != "leader-1" {
		t.Errorf("expected leader leader-1, got %s", party.LeaderID)
	}
	if !party.IsOpen {
		t.Error("expected party to be open")
	}
	if party.MaxSize != 5 {
		t.Errorf("expected max size 5, got %d", party.MaxSize)
	}
	if !party.IsReady {
		t.Error("expected party to be ready")
	}
	if !party.CreatedAt.Equal(now) {
		t.Errorf("expected created at %v, got %v", now, party.CreatedAt)
	}
}
