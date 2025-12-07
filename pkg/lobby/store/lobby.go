package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/argus-labs/world-engine/pkg/lobby/types"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// LobbyStore manages lobby storage with indexes for efficient lookups.
// From ADR-030:
//   - lobbies_by_match: Map<match_id, Lobby> (primary storage)
//   - lobbies_by_state: Map<state, Set<match_id>>
//   - lobby_by_party: Map<party_id, match_id>
//
// Note: match_id is the primary key - there is no separate lobby_id.
type LobbyStore struct {
	mu sync.RWMutex

	// Primary storage - O(1) lookup by match_id
	lobbiesByMatch map[string]*types.Lobby

	// Index by state - for listing/searching
	lobbiesByState map[types.LobbyState]map[string]bool

	// Index by party - find lobby by party
	lobbyByParty map[string]string

	// Counter for deterministic ID generation (used for manual lobbies)
	counter uint64
}

// NewLobbyStore creates a new lobby store.
func NewLobbyStore() *LobbyStore {
	return &LobbyStore{
		lobbiesByMatch: make(map[string]*types.Lobby),
		lobbiesByState: make(map[types.LobbyState]map[string]bool),
		lobbyByParty:   make(map[string]string),
	}
}

// Create creates a manual lobby (not from matchmaking).
// Generates a unique ID for the lobby using the internal counter.
func (s *LobbyStore) Create(
	hostPartyID string,
	minPlayers, maxPlayers int,
	config map[string]any,
	now time.Time,
) (*types.Lobby, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if party is already in a lobby
	if _, exists := s.lobbyByParty[hostPartyID]; exists {
		return nil, fmt.Errorf("party %s is already in a lobby", hostPartyID)
	}

	// Generate a unique lobby ID
	s.counter++
	lobbyID := fmt.Sprintf("lobby-%d", s.counter)

	lobby := &types.Lobby{
		MatchID:     lobbyID, // For manual lobbies, match_id is generated
		HostPartyID: hostPartyID,
		Parties:     []string{hostPartyID},
		State:       types.LobbyStateWaiting,
		Config:      config,
		MinPlayers:  minPlayers,
		MaxPlayers:  maxPlayers,
		CreatedAt:   now,
	}

	s.addLobby(lobby)
	return lobby, nil
}

// CreateFromMatch creates a lobby from a match received from Matchmaking Shard.
// The match_id becomes the lobby's primary key.
func (s *LobbyStore) CreateFromMatch(
	matchID string,
	matchProfileName string,
	teams []types.LobbyTeam,
	config map[string]any,
	matchmakingAddress *microv1.ServiceAddress,
	targetAddress *microv1.ServiceAddress,
	now time.Time,
) (*types.Lobby, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if match already has a lobby
	if _, exists := s.lobbiesByMatch[matchID]; exists {
		return nil, fmt.Errorf("match %s already has a lobby", matchID)
	}

	// Collect all party IDs and check none are in other lobbies
	partyIDs := make([]string, 0)
	for _, team := range teams {
		for _, partyID := range team.PartyIDs {
			if _, exists := s.lobbyByParty[partyID]; exists {
				return nil, fmt.Errorf("party %s is already in a lobby", partyID)
			}
			partyIDs = append(partyIDs, partyID)
		}
	}

	// Use first party as host
	var hostPartyID string
	if len(partyIDs) > 0 {
		hostPartyID = partyIDs[0]
	}

	// Calculate player counts from teams
	totalPlayers := 0
	for _, team := range teams {
		totalPlayers += len(team.PartyIDs)
	}

	s.counter++
	lobby := &types.Lobby{
		MatchID:            matchID,
		HostPartyID:        hostPartyID,
		Parties:            partyIDs,
		Teams:              teams,
		State:              types.LobbyStateWaiting,
		MatchProfileName:   matchProfileName,
		MatchmakingAddress: matchmakingAddress,
		TargetAddress:      targetAddress,
		Config:             config,
		MinPlayers:         totalPlayers,
		MaxPlayers:         totalPlayers,
		CreatedAt:          now,
	}

	s.addLobby(lobby)
	return lobby, nil
}

// addLobby adds a lobby to all indexes (internal, must hold lock).
func (s *LobbyStore) addLobby(lobby *types.Lobby) {
	s.lobbiesByMatch[lobby.MatchID] = lobby
	s.addToStateIndex(lobby.MatchID, lobby.State)

	for _, partyID := range lobby.Parties {
		s.lobbyByParty[partyID] = lobby.MatchID
	}
}

// addToStateIndex adds a lobby to the state index.
func (s *LobbyStore) addToStateIndex(matchID string, state types.LobbyState) {
	if s.lobbiesByState[state] == nil {
		s.lobbiesByState[state] = make(map[string]bool)
	}
	s.lobbiesByState[state][matchID] = true
}

// removeFromStateIndex removes a lobby from the state index.
func (s *LobbyStore) removeFromStateIndex(matchID string, state types.LobbyState) {
	if s.lobbiesByState[state] != nil {
		delete(s.lobbiesByState[state], matchID)
	}
}

// Get returns a lobby by match_id.
func (s *LobbyStore) Get(matchID string) (*types.Lobby, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	lobby, ok := s.lobbiesByMatch[matchID]
	return lobby, ok
}

// GetByParty returns the lobby for a given party.
func (s *LobbyStore) GetByParty(partyID string) (*types.Lobby, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matchID, ok := s.lobbyByParty[partyID]
	if !ok {
		return nil, false
	}
	return s.lobbiesByMatch[matchID], true
}

// GetByState returns all lobbies in a given state.
func (s *LobbyStore) GetByState(state types.LobbyState) []*types.Lobby {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matchIDs := s.lobbiesByState[state]
	lobbies := make([]*types.Lobby, 0, len(matchIDs))
	for matchID := range matchIDs {
		if lobby, ok := s.lobbiesByMatch[matchID]; ok {
			lobbies = append(lobbies, lobby)
		}
	}
	return lobbies
}

// AddParty adds a party to a lobby.
func (s *LobbyStore) AddParty(matchID, partyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lobby, ok := s.lobbiesByMatch[matchID]
	if !ok {
		return fmt.Errorf("lobby %s not found", matchID)
	}

	if !lobby.CanJoin() {
		return fmt.Errorf("lobby %s is not accepting new parties", matchID)
	}

	if _, exists := s.lobbyByParty[partyID]; exists {
		return fmt.Errorf("party %s is already in a lobby", partyID)
	}

	lobby.Parties = append(lobby.Parties, partyID)
	s.lobbyByParty[partyID] = matchID

	return nil
}

// RemoveParty removes a party from a lobby.
// Returns true if lobby was closed (host left and no one remaining).
func (s *LobbyStore) RemoveParty(matchID, partyID string) (lobbyClosed bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	lobby, ok := s.lobbiesByMatch[matchID]
	if !ok {
		return false, fmt.Errorf("lobby %s not found", matchID)
	}

	if !lobby.HasParty(partyID) {
		return false, fmt.Errorf("party %s is not in lobby %s", partyID, matchID)
	}

	// Remove from parties list
	newParties := make([]string, 0, len(lobby.Parties)-1)
	for _, p := range lobby.Parties {
		if p != partyID {
			newParties = append(newParties, p)
		}
	}
	lobby.Parties = newParties

	// Remove from teams if present
	for i := range lobby.Teams {
		newPartyIDs := make([]string, 0)
		for _, pid := range lobby.Teams[i].PartyIDs {
			if pid != partyID {
				newPartyIDs = append(newPartyIDs, pid)
			}
		}
		lobby.Teams[i].PartyIDs = newPartyIDs
	}

	// Remove from index
	delete(s.lobbyByParty, partyID)

	// If lobby is empty, close it
	if len(lobby.Parties) == 0 {
		s.deleteLobby(lobby)
		return true, nil
	}

	// If host left, promote next party
	if lobby.HostPartyID == partyID {
		lobby.HostPartyID = lobby.Parties[0]
	}

	return false, nil
}

// SetState changes the lobby state.
func (s *LobbyStore) SetState(matchID string, state types.LobbyState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lobby, ok := s.lobbiesByMatch[matchID]
	if !ok {
		return fmt.Errorf("lobby %s not found", matchID)
	}

	oldState := lobby.State
	lobby.State = state

	// Update state index
	s.removeFromStateIndex(matchID, oldState)
	s.addToStateIndex(matchID, state)

	return nil
}

// SetPartyConnected updates a party's connection status.
// If connected=false, adds to disconnected list. If connected=true, removes from it.
func (s *LobbyStore) SetPartyConnected(matchID, partyID string, connected bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lobby, ok := s.lobbiesByMatch[matchID]
	if !ok {
		return fmt.Errorf("lobby %s not found", matchID)
	}

	if connected {
		// Remove from disconnected list
		for i, p := range lobby.DisconnectedParties {
			if p == partyID {
				lobby.DisconnectedParties = append(lobby.DisconnectedParties[:i], lobby.DisconnectedParties[i+1:]...)
				return nil
			}
		}
	} else {
		// Add to disconnected list if not already there
		for _, p := range lobby.DisconnectedParties {
			if p == partyID {
				return nil // Already marked
			}
		}
		lobby.DisconnectedParties = append(lobby.DisconnectedParties, partyID)
	}

	return nil
}

// SetStartedAt sets the started timestamp.
func (s *LobbyStore) SetStartedAt(matchID string, startedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lobby, ok := s.lobbiesByMatch[matchID]
	if !ok {
		return fmt.Errorf("lobby %s not found", matchID)
	}

	lobby.StartedAt = &startedAt
	return nil
}

// UpdateHeartbeat updates the last heartbeat timestamp.
func (s *LobbyStore) UpdateHeartbeat(matchID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lobby, ok := s.lobbiesByMatch[matchID]
	if !ok {
		return fmt.Errorf("lobby %s not found", matchID)
	}

	lobby.LastHeartbeat = &now
	return nil
}

// Delete removes a lobby.
func (s *LobbyStore) Delete(matchID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	lobby, ok := s.lobbiesByMatch[matchID]
	if !ok {
		return false
	}

	s.deleteLobby(lobby)
	return true
}

// deleteLobby removes a lobby from all indexes (internal, must hold lock).
func (s *LobbyStore) deleteLobby(lobby *types.Lobby) {
	delete(s.lobbiesByMatch, lobby.MatchID)
	s.removeFromStateIndex(lobby.MatchID, lobby.State)

	for _, partyID := range lobby.Parties {
		delete(s.lobbyByParty, partyID)
	}
}

// All returns all lobbies.
func (s *LobbyStore) All() []*types.Lobby {
	s.mu.RLock()
	defer s.mu.RUnlock()

	lobbies := make([]*types.Lobby, 0, len(s.lobbiesByMatch))
	for _, l := range s.lobbiesByMatch {
		lobbies = append(lobbies, l)
	}
	return lobbies
}

// Count returns the total number of lobbies.
func (s *LobbyStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.lobbiesByMatch)
}

// CountByState returns the number of lobbies in a given state.
func (s *LobbyStore) CountByState(state types.LobbyState) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.lobbiesByState[state])
}

// Clear removes all lobbies.
func (s *LobbyStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lobbiesByMatch = make(map[string]*types.Lobby)
	s.lobbiesByState = make(map[types.LobbyState]map[string]bool)
	s.lobbyByParty = make(map[string]string)
}

// Restore adds a lobby to the store (used for snapshot restoration).
func (s *LobbyStore) Restore(lobby *types.Lobby) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.addLobby(lobby)
}

// GetCounter returns the current counter value.
func (s *LobbyStore) GetCounter() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.counter
}

// SetCounter sets the counter value (used for snapshot restoration).
func (s *LobbyStore) SetCounter(val uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter = val
}

// GetZombieLobbies returns lobbies in in_game state that haven't received a heartbeat
// within the specified timeout duration.
func (s *LobbyStore) GetZombieLobbies(now time.Time, timeout time.Duration) []*types.Lobby {
	s.mu.RLock()
	defer s.mu.RUnlock()

	zombies := make([]*types.Lobby, 0)
	for matchID := range s.lobbiesByState[types.LobbyStateInGame] {
		lobby := s.lobbiesByMatch[matchID]
		if lobby.LastHeartbeat == nil {
			// No heartbeat ever received, check started time
			if lobby.StartedAt != nil && now.Sub(*lobby.StartedAt) > timeout {
				zombies = append(zombies, lobby)
			}
		} else if now.Sub(*lobby.LastHeartbeat) > timeout {
			zombies = append(zombies, lobby)
		}
	}
	return zombies
}
