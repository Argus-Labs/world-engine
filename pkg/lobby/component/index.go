package component

// LobbyIndexComponent provides O(1) lookups for lobbies.
// This is a singleton component - only one entity should have it.
type LobbyIndexComponent struct {
	// MatchIDToEntity maps MatchID -> EntityID for O(1) lookup
	MatchIDToEntity map[string]uint32 `json:"match_id_to_entity"`

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
func (idx *LobbyIndexComponent) AddLobby(matchID string, entityID uint32) {
	idx.Init()
	idx.MatchIDToEntity[matchID] = entityID
	idx.InGameLobbies = append(idx.InGameLobbies, matchID)
}

// RemoveLobby removes a lobby from the index.
func (idx *LobbyIndexComponent) RemoveLobby(matchID string) {
	delete(idx.MatchIDToEntity, matchID)
	idx.InGameLobbies = removeFromSlice(idx.InGameLobbies, matchID)
}

// ConfigComponent stores lobby configuration.
type ConfigComponent struct {
	// MatchmakingShardID is the shard ID for matchmaking (for receiving matches and sending backfill)
	MatchmakingShardID string `json:"matchmaking_shard_id,omitempty"`

	// MatchmakingRegion is the region for the matchmaking shard (for cross-shard).
	MatchmakingRegion string `json:"matchmaking_region,omitempty"`

	// MatchmakingOrganization is the organization for the matchmaking shard (for cross-shard).
	MatchmakingOrganization string `json:"matchmaking_organization,omitempty"`

	// MatchmakingProject is the project for the matchmaking shard (for cross-shard).
	MatchmakingProject string `json:"matchmaking_project,omitempty"`

	// GameShardID is the shard ID for the game shard (for sending game starts)
	GameShardID string `json:"game_shard_id,omitempty"`

	// GameRegion is the region for the game shard (for cross-shard).
	GameRegion string `json:"game_region,omitempty"`

	// GameOrganization is the organization for the game shard (for cross-shard).
	GameOrganization string `json:"game_organization,omitempty"`

	// GameProject is the project for the game shard (for cross-shard).
	GameProject string `json:"game_project,omitempty"`

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
