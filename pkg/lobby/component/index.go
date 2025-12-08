package component

// PartyIndexComponent provides O(1) lookups for parties.
// This is a singleton component - only one entity should have it.
type PartyIndexComponent struct {
	// PartyIDToEntity maps PartyID -> EntityID for O(1) lookup
	PartyIDToEntity map[string]uint32 `json:"party_id_to_entity"`

	// PlayerToParty maps PlayerID -> PartyID (one party per player)
	PlayerToParty map[string]string `json:"player_to_party"`

	// LobbyToParties maps LobbyID -> []PartyID
	LobbyToParties map[string][]string `json:"lobby_to_parties"`
}

// Name returns the component name for ECS registration.
func (PartyIndexComponent) Name() string { return "lobby_party_index" }

// Init initializes the maps if nil.
func (idx *PartyIndexComponent) Init() {
	if idx.PartyIDToEntity == nil {
		idx.PartyIDToEntity = make(map[string]uint32)
	}
	if idx.PlayerToParty == nil {
		idx.PlayerToParty = make(map[string]string)
	}
	if idx.LobbyToParties == nil {
		idx.LobbyToParties = make(map[string][]string)
	}
}

// GetEntityID returns the entity ID for a party.
func (idx *PartyIndexComponent) GetEntityID(partyID string) (uint32, bool) {
	eid, exists := idx.PartyIDToEntity[partyID]
	return eid, exists
}

// GetPartyByPlayer returns the party ID for a player.
func (idx *PartyIndexComponent) GetPartyByPlayer(playerID string) (string, bool) {
	partyID, exists := idx.PlayerToParty[playerID]
	return partyID, exists
}

// HasPlayer checks if a player already has a party.
func (idx *PartyIndexComponent) HasPlayer(playerID string) bool {
	_, exists := idx.PlayerToParty[playerID]
	return exists
}

// GetPartiesByLobby returns all party IDs in a lobby.
func (idx *PartyIndexComponent) GetPartiesByLobby(lobbyID string) []string {
	return idx.LobbyToParties[lobbyID]
}

// AddParty adds a party to the index.
func (idx *PartyIndexComponent) AddParty(partyID string, entityID uint32, memberIDs []string) {
	idx.Init()
	idx.PartyIDToEntity[partyID] = entityID
	for _, playerID := range memberIDs {
		idx.PlayerToParty[playerID] = partyID
	}
}

// RemoveParty removes a party from the index.
func (idx *PartyIndexComponent) RemoveParty(partyID string, memberIDs []string) {
	delete(idx.PartyIDToEntity, partyID)
	for _, playerID := range memberIDs {
		delete(idx.PlayerToParty, playerID)
	}
}

// SetPartyLobby updates the lobby assignment for a party.
func (idx *PartyIndexComponent) SetPartyLobby(partyID string, lobbyID string) {
	idx.Init()
	// Remove from old lobby if any
	for lid, parties := range idx.LobbyToParties {
		for i, pid := range parties {
			if pid == partyID {
				idx.LobbyToParties[lid] = append(parties[:i], parties[i+1:]...)
				break
			}
		}
	}
	// Add to new lobby
	if lobbyID != "" {
		idx.LobbyToParties[lobbyID] = append(idx.LobbyToParties[lobbyID], partyID)
	}
}

// AddPlayerToParty adds a player to the party index.
func (idx *PartyIndexComponent) AddPlayerToParty(playerID string, partyID string) {
	idx.Init()
	idx.PlayerToParty[playerID] = partyID
}

// RemovePlayerFromParty removes a player from the party index.
func (idx *PartyIndexComponent) RemovePlayerFromParty(playerID string) {
	delete(idx.PlayerToParty, playerID)
}

// LobbyIndexComponent provides O(1) lookups for lobbies.
// This is a singleton component - only one entity should have it.
type LobbyIndexComponent struct {
	// MatchIDToEntity maps MatchID -> EntityID for O(1) lookup
	MatchIDToEntity map[string]uint32 `json:"match_id_to_entity"`

	// ActiveLobbies is a list of match IDs in waiting/ready state
	ActiveLobbies []string `json:"active_lobbies"`

	// InGameLobbies is a list of match IDs in in_game state
	InGameLobbies []string `json:"in_game_lobbies"`
}

// Name returns the component name for ECS registration.
func (LobbyIndexComponent) Name() string { return "lobby_index" }

// Init initializes the maps if nil.
func (idx *LobbyIndexComponent) Init() {
	if idx.MatchIDToEntity == nil {
		idx.MatchIDToEntity = make(map[string]uint32)
	}
	if idx.ActiveLobbies == nil {
		idx.ActiveLobbies = []string{}
	}
	if idx.InGameLobbies == nil {
		idx.InGameLobbies = []string{}
	}
}

// GetEntityID returns the entity ID for a lobby.
func (idx *LobbyIndexComponent) GetEntityID(matchID string) (uint32, bool) {
	eid, exists := idx.MatchIDToEntity[matchID]
	return eid, exists
}

// AddLobby adds a lobby to the index.
func (idx *LobbyIndexComponent) AddLobby(matchID string, entityID uint32, state LobbyState) {
	idx.Init()
	idx.MatchIDToEntity[matchID] = entityID
	if state == LobbyStateWaiting || state == LobbyStateReady {
		idx.ActiveLobbies = append(idx.ActiveLobbies, matchID)
	} else if state == LobbyStateInGame {
		idx.InGameLobbies = append(idx.InGameLobbies, matchID)
	}
}

// RemoveLobby removes a lobby from the index.
func (idx *LobbyIndexComponent) RemoveLobby(matchID string) {
	delete(idx.MatchIDToEntity, matchID)
	idx.ActiveLobbies = removeFromSlice(idx.ActiveLobbies, matchID)
	idx.InGameLobbies = removeFromSlice(idx.InGameLobbies, matchID)
}

// UpdateLobbyState updates the lobby state in the index.
func (idx *LobbyIndexComponent) UpdateLobbyState(matchID string, oldState, newState LobbyState) {
	idx.Init()
	// Remove from old list
	if oldState == LobbyStateWaiting || oldState == LobbyStateReady {
		idx.ActiveLobbies = removeFromSlice(idx.ActiveLobbies, matchID)
	} else if oldState == LobbyStateInGame {
		idx.InGameLobbies = removeFromSlice(idx.InGameLobbies, matchID)
	}
	// Add to new list
	if newState == LobbyStateWaiting || newState == LobbyStateReady {
		idx.ActiveLobbies = append(idx.ActiveLobbies, matchID)
	} else if newState == LobbyStateInGame {
		idx.InGameLobbies = append(idx.InGameLobbies, matchID)
	}
}

// ConfigComponent stores lobby configuration.
type ConfigComponent struct {
	// MatchmakingShardID is the shard ID for matchmaking (for receiving matches)
	MatchmakingShardID string `json:"matchmaking_shard_id,omitempty"`

	// GameShardID is the shard ID for the game shard (for sending game starts)
	GameShardID string `json:"game_shard_id,omitempty"`

	// DefaultMaxPartySize is the default max party size.
	DefaultMaxPartySize int `json:"default_max_party_size"`

	// HeartbeatTimeoutSeconds is how long before a lobby is considered stale.
	HeartbeatTimeoutSeconds int64 `json:"heartbeat_timeout_seconds"`
}

// Name returns the component name for ECS registration.
func (ConfigComponent) Name() string { return "lobby_config" }

// removeFromSlice removes a value from a slice.
func removeFromSlice(slice []string, value string) []string {
	for i, v := range slice {
		if v == value {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
