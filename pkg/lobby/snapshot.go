package lobby

import (
	"encoding/json"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/pkg/lobby/types"
)

// snapshotData represents the serialized state.
type snapshotData struct {
	Parties      []*types.Party `json:"parties"`
	PartyCounter uint64         `json:"party_counter"`
	Lobbies      []*types.Lobby `json:"lobbies"`
	LobbyCounter uint64         `json:"lobby_counter"`
}

// serialize converts the current state to bytes.
func (l *lobby) serialize() ([]byte, error) {
	data := snapshotData{
		Parties:      l.parties.All(),
		PartyCounter: l.parties.GetCounter(),
		Lobbies:      l.lobbies.All(),
		LobbyCounter: l.lobbies.GetCounter(),
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal snapshot")
	}

	return bytes, nil
}

// deserialize restores state from bytes.
func (l *lobby) deserialize(data []byte) error {
	var snapshot snapshotData
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return eris.Wrap(err, "failed to unmarshal snapshot")
	}

	// Clear existing state
	l.parties.Clear()
	l.lobbies.Clear()

	// Restore parties
	for _, party := range snapshot.Parties {
		l.parties.Restore(party)
	}
	l.parties.SetCounter(snapshot.PartyCounter)

	// Restore lobbies
	for _, lobby := range snapshot.Lobbies {
		l.lobbies.Restore(lobby)
	}
	l.lobbies.SetCounter(snapshot.LobbyCounter)

	return nil
}
