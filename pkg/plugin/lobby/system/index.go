package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/lobby/component"
)

// lookupIndex provides O(1) lookups over lobby and player entities.
//
// Deliberately NOT a component. Every map here is derived from data the entities already hold, so
// storing it in ECS would persist a conclusion beside the facts it came from — and any drift between
// the two would then survive every restore instead of being repaired by one. Keeping it out also
// means the maps can stay maps: component fields must copy cleanly, plugin state need not.
//
// Not guarded by a mutex: cardinal runs systems sequentially on the tick goroutine (see
// internal/ecs/world.go Tick), so nothing here is ever touched concurrently.
type lookupIndex struct {
	// LobbyIDToEntity maps LobbyID -> EntityID for O(1) lookup
	LobbyIDToEntity map[string]uint32

	// InviteCodeToLobby maps InviteCode -> LobbyID for join lookups
	InviteCodeToLobby map[string]string

	// PlayerToLobby maps PlayerID -> LobbyID for "my lobby" lookups
	PlayerToLobby map[string]string

	// PlayerToTeam maps PlayerID -> TeamID for O(1) team lookup
	PlayerToTeam map[string]string

	// PlayerToEntity maps PlayerID -> EntityID for O(1) player entity lookup
	PlayerToEntity map[string]uint32

	// PlayerDeadline maps PlayerID -> Unix timestamp when player will be kicked if no heartbeat
	PlayerDeadline map[string]int64

	// LobbyPlayerCount maps LobbyID -> player count for O(1) count lookup
	LobbyPlayerCount map[string]int
}

// index is the live lookup table for this process.
//
//nolint:gochecknoglobals // derived state owned by this package; cardinal has no per-world store
var index lookupIndex

// indexBuilt guards the rebuild: set by rebuildIndex, cleared by InitSystem, which runs both on
// boot and inside World.reset().
//
//nolint:gochecknoglobals // see index
var indexBuilt bool

// lobbyRow and playerRow carry an entity and its component to the rebuild, so the rebuild does not
// have to be written once per system state type.
type lobbyRow struct {
	entityID cardinal.EntityID
	lobby    component.LobbyComponent
}

type playerRow struct {
	entityID cardinal.EntityID
	player   component.PlayerComponent
}

// rebuildIndex reconstructs every map from entities.
//
// Runs on the first tick rather than in the init system: cardinal calls Init() BEFORE restore, and
// restore replaces the world wholesale (worldState.fromProto reassigns ws.archetypes), so anything an
// init system builds is discarded on a restored shard. The first tick is the earliest point at which
// the entities are the ones that will actually be used.
//
// Deadlines are reset to now+timeout rather than carried over, because a deadline measures client
// liveness and clients cannot heartbeat while the process is down. Restoring the stored value would
// evict every player in every lobby after any outage longer than the timeout.
func rebuildIndex(lobbies []lobbyRow, players []playerRow, now, heartbeatTimeout int64) {
	index = lookupIndex{}
	index.Init()

	for _, row := range lobbies {
		index.AddLobby(row.lobby.ID, uint32(row.entityID), row.lobby.InviteCode)
	}
	for _, row := range players {
		index.AddPlayerToLobby(
			row.player.PlayerID,
			row.player.LobbyID,
			row.player.TeamID,
			uint32(row.entityID),
			now+heartbeatTimeout,
		)
	}
	indexBuilt = true
}

// Init initializes the maps if nil.
func (idx *lookupIndex) Init() {
	if idx.LobbyIDToEntity == nil {
		idx.LobbyIDToEntity = make(map[string]uint32)
	}
	if idx.InviteCodeToLobby == nil {
		idx.InviteCodeToLobby = make(map[string]string)
	}
	if idx.PlayerToLobby == nil {
		idx.PlayerToLobby = make(map[string]string)
	}
	if idx.PlayerToTeam == nil {
		idx.PlayerToTeam = make(map[string]string)
	}
	if idx.PlayerToEntity == nil {
		idx.PlayerToEntity = make(map[string]uint32)
	}
	if idx.PlayerDeadline == nil {
		idx.PlayerDeadline = make(map[string]int64)
	}
	if idx.LobbyPlayerCount == nil {
		idx.LobbyPlayerCount = make(map[string]int)
	}
}

// GetEntityID returns the entity ID for a lobby.
func (idx *lookupIndex) GetEntityID(lobbyID string) (uint32, bool) {
	eid, exists := idx.LobbyIDToEntity[lobbyID]
	return eid, exists
}

// GetLobbyByInviteCode returns the lobby ID for an invite code.
func (idx *lookupIndex) GetLobbyByInviteCode(inviteCode string) (string, bool) {
	lobbyID, exists := idx.InviteCodeToLobby[inviteCode]
	return lobbyID, exists
}

// InviteCodeCount returns how many invite codes the index currently knows.
// Logged on a rejected join to separate "this one code is missing" from "the index is
// empty", which look identical to the player.
func (idx *lookupIndex) InviteCodeCount() int {
	return len(idx.InviteCodeToLobby)
}

// GetPlayerLobby returns the lobby ID for a player.
func (idx *lookupIndex) GetPlayerLobby(playerID string) (string, bool) {
	lobbyID, exists := idx.PlayerToLobby[playerID]
	return lobbyID, exists
}

// AddLobby adds a lobby to the index.
func (idx *lookupIndex) AddLobby(lobbyID string, entityID uint32, inviteCode string) {
	idx.Init()
	idx.LobbyIDToEntity[lobbyID] = entityID
	if inviteCode != "" {
		idx.InviteCodeToLobby[inviteCode] = lobbyID
	}
}

// RemoveLobby removes a lobby from the index.
func (idx *lookupIndex) RemoveLobby(lobbyID string, inviteCode string) {
	delete(idx.LobbyIDToEntity, lobbyID)
	if inviteCode != "" {
		delete(idx.InviteCodeToLobby, inviteCode)
	}
}

// AddPlayerToLobby maps a player to a lobby, team, entity ID, and sets their deadline.
func (idx *lookupIndex) AddPlayerToLobby(playerID, lobbyID, teamID string, entityID uint32, deadline int64) {
	idx.Init()
	idx.PlayerToLobby[playerID] = lobbyID
	idx.PlayerToTeam[playerID] = teamID
	idx.PlayerToEntity[playerID] = entityID
	idx.PlayerDeadline[playerID] = deadline
	idx.LobbyPlayerCount[lobbyID]++
}

// RemovePlayerFromLobby removes a player's lobby mapping, team mapping, entity mapping, and deadline.
func (idx *lookupIndex) RemovePlayerFromLobby(playerID string) {
	if lobbyID, exists := idx.PlayerToLobby[playerID]; exists {
		idx.LobbyPlayerCount[lobbyID]--
		if idx.LobbyPlayerCount[lobbyID] <= 0 {
			delete(idx.LobbyPlayerCount, lobbyID)
		}
	}
	delete(idx.PlayerToLobby, playerID)
	delete(idx.PlayerToTeam, playerID)
	delete(idx.PlayerToEntity, playerID)
	delete(idx.PlayerDeadline, playerID)
}

// GetPlayerEntityID returns the entity ID for a player.
func (idx *lookupIndex) GetPlayerEntityID(playerID string) (uint32, bool) {
	eid, exists := idx.PlayerToEntity[playerID]
	return eid, exists
}

// UpdatePlayerDeadline updates the deadline for a player.
func (idx *lookupIndex) UpdatePlayerDeadline(playerID string, deadline int64) {
	idx.PlayerDeadline[playerID] = deadline
}

// GetPlayerDeadline returns the deadline for a player.
func (idx *lookupIndex) GetPlayerDeadline(playerID string) (int64, bool) {
	deadline, exists := idx.PlayerDeadline[playerID]
	return deadline, exists
}

// GetPlayerTeam returns the team ID for a player.
func (idx *lookupIndex) GetPlayerTeam(playerID string) (string, bool) {
	teamID, exists := idx.PlayerToTeam[playerID]
	return teamID, exists
}

// UpdatePlayerTeam updates the team ID for a player.
func (idx *lookupIndex) UpdatePlayerTeam(playerID, teamID string) {
	idx.PlayerToTeam[playerID] = teamID
}

// GetLobbyPlayerCount returns the player count for a lobby.
func (idx *lookupIndex) GetLobbyPlayerCount(lobbyID string) int {
	return idx.LobbyPlayerCount[lobbyID]
}

// HasPlayer returns true if player exists in the index (O(1)).
func (idx *lookupIndex) HasPlayer(playerID string) bool {
	_, exists := idx.PlayerToLobby[playerID]
	return exists
}

// UpdateInviteCode updates the invite code for a lobby.
func (idx *lookupIndex) UpdateInviteCode(lobbyID, oldCode, newCode string) {
	if oldCode != "" {
		delete(idx.InviteCodeToLobby, oldCode)
	}
	if newCode != "" {
		idx.InviteCodeToLobby[newCode] = lobbyID
	}
}
