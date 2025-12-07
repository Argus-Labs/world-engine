# Lobby Shard

The Lobby Shard manages party formation, pre-game lobbies, and coordinates between players, matchmaking, and game shards.

## Architecture Overview

```
┌─────────────┐     ┌─────────────┐     ┌─────────────────┐     ┌─────────────┐
│   Client    │ ──→ │   Lobby     │ ──→ │  Matchmaking    │     │ Game Shard  │
│             │     │   Shard     │ ←── │     Shard       │     │ (Cardinal)  │
└─────────────┘     └──────┬──────┘     └─────────────────┘     └──────┬──────┘
                           │                                           │
                           └───────────────────────────────────────────┘
                                    (lobby ↔ game communication)
```

**Lobby Shard is the single entry point for players.** It handles:
- Party creation and management
- Forwarding matchmaking requests (after validation)
- Receiving matches from Matchmaking Shard
- Ready-up and game start coordination
- Communication with Game Shard

## Core Concepts

### Party

A **Party** is a group of 1+ players who queue and play together. Parties are the atomic unit for matchmaking - they are never split across teams.

```go
type Party struct {
    ID        string    // Server-generated UUID
    LeaderID  string    // Player who controls the party
    Members   []string  // All player IDs in the party
    JoinCode  string    // Human-readable code for joining (e.g., "ABC123")
    IsOpen    bool      // Whether others can join via code
    MaxSize   int       // Maximum party size
    LobbyID   string    // Set when party is in a lobby
    IsReady   bool      // Ready status in lobby
    CreatedAt time.Time
}
```

### Lobby

A **Lobby** is a pre-game room where parties gather before a match starts.

```go
type Lobby struct {
    ID                 string       // Server-generated UUID
    HostPartyID        string       // Party that controls the lobby
    Parties            []string     // All party IDs in the lobby
    Teams              []LobbyTeam  // Team assignments
    State              LobbyState   // waiting, ready, in_game
    MatchID            string       // If created from matchmaking
    MatchmakingAddress *ServiceAddress // For backfill requests
    ConnectionInfo     *ConnectionInfo // Game server connection details
}
```

## Party System

### Party ID Generation

**Party IDs are always server-generated UUIDs.** This prevents:
- Session hijacking (attacker using victim's party ID)
- Impersonation attacks
- Griefing via party ID spoofing

### Join Methods

#### 1. Direct Invite
```
Leader → Lobby.invite-to-party(party_id, invitee_player_id)
         └─→ Creates pending invite

Invitee → Lobby.accept-invite(invite_id)
          └─→ Server validates invite
          └─→ Adds player to party
```

#### 2. Join Code
```
Leader → Lobby.create-party(player_id, is_open=true)
         └─→ Server generates:
             - party_id: "550e8400-e29b-41d4-a716-446655440000" (UUID)
             - join_code: "ABC123" (human-friendly, 6 chars)

Friend → Lobby.join-by-code(join_code, player_id)
         └─→ Server looks up party_id from code
         └─→ Validates party is open and has space
         └─→ Adds player to party
```

### Join Code Best Practices

- **Format**: 6 alphanumeric characters (uppercase)
- **Excluded chars**: `0/O`, `1/I/L` (avoid confusion)
- **Charset**: `ABCDEFGHJKMNPQRSTUVWXYZ23456789` (32 chars)
- **Expiration**: Code expires when party disbands
- **Rate limiting**: Prevent brute-force guessing

## Flows

### Flow 1: Party Formation → Matchmaking → Game

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. PARTY FORMATION                                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   Player A → Lobby.create-party                                  │
│              └─→ Returns party_id + join_code                    │
│                                                                  │
│   Player B → Lobby.join-by-code(join_code)                      │
│              └─→ Added to party                                  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ 2. QUEUE FOR MATCH                                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   Player A → Lobby.queue-for-match(party_id, profile_name)      │
│              └─→ Lobby verifies A is party leader                │
│              └─→ Lobby → Matchmaking.create-ticket               │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ 3. MATCH FOUND                                                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   Matchmaking → Lobby.matchmaking.match                          │
│                 └─→ Lobby creates lobby with parties             │
│                 └─→ Lobby notifies players                       │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ 4. READY UP                                                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   All parties → Lobby.set-ready(party_id, lobby_id)             │
│                 └─→ When all ready: state → "ready"              │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ 5. GAME START                                                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   Lobby → Game Shard (start game with player list)               │
│           └─→ Includes lobby_address for callbacks               │
│   Lobby state → "in_game"                                        │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ 6. GAME END                                                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   Game Shard → Lobby.end-match                                   │
│                └─→ Lobby deleted                                 │
│                └─→ Parties disbanded or returned to menu         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Flow 2: Manual Lobby (Custom Games)

```
Player A → Lobby.create-lobby(party_id, config)
           └─→ Returns lobby_id + join_code

Player B → Lobby.join-lobby-by-code(join_code, party_id)
           └─→ Added to lobby

All parties → Lobby.set-ready(party_id, lobby_id)

Host → Lobby.start-game(lobby_id)
       └─→ Forwards to Game Shard
```

## Lobby State Machine

### States

| State | Description |
|-------|-------------|
| `waiting` | Players in lobby, waiting for ready check |
| `ready` | All players/parties confirmed ready |
| `in_game` | Game is active |

### State Transitions

```
waiting → ready → in_game → (deleted)
    ↑       │
    └───────┘
   (unready/leave)
```

1. **waiting → ready**: All players/parties confirm ready
2. **ready → waiting**: Player leaves or unreadies
3. **ready → in_game**: Game starts
4. **in_game → deleted**: Game finishes (Game Shard sends EndMatch)

### Actions per State

#### waiting

| Action | Notes |
|--------|-------|
| Join lobby | New players can join |
| Leave lobby | Players can disconnect/leave |
| Ready up | Player confirms ready |
| Unready | Player cancels ready |

#### ready

| Action | Notes |
|--------|-------|
| Leave lobby | Returns lobby to waiting |
| Unready | Returns lobby to waiting |
| Start game | Triggers transition to in_game |

#### in_game

| Action | Notes |
|--------|-------|
| Disconnect | Player disconnects (via SetPlayerStatus command) |
| Reconnect | Player reconnects (via SetPlayerStatus command) |
| Heartbeat | Game Shard sends heartbeat to keep lobby alive |
| Request backfill | Game Shard calls `backfill.request` endpoint |
| Cancel backfill | Game Shard calls `backfill.cancel` endpoint |
| End game | Game Shard sends EndMatch → lobby deleted |

## Commands (Client → Lobby)

### Party Commands

| Command | Description | Payload |
|---------|-------------|---------|
| `create-party` | Create a new party | `player_id`, `is_open`, `max_size` |
| `join-by-code` | Join party via code | `join_code`, `player_id` |
| `leave-party` | Leave current party | `party_id`, `player_id` |
| `invite-to-party` | Invite player | `party_id`, `invitee_id` |
| `accept-invite` | Accept party invite | `invite_id`, `player_id` |
| `kick-from-party` | Kick member (leader only) | `party_id`, `player_id` |
| `promote-leader` | Transfer leadership | `party_id`, `new_leader_id` |
| `set-party-open` | Toggle open/closed | `party_id`, `is_open` |

### Matchmaking Commands

| Command | Description | Payload |
|---------|-------------|---------|
| `queue-for-match` | Enter matchmaking | `party_id`, `profile_name` |
| `cancel-queue` | Leave matchmaking | `party_id` |

### Lobby Commands

| Command | Description | Payload |
|---------|-------------|---------|
| `create-lobby` | Create manual lobby | `party_id`, `config` |
| `join-lobby` | Join existing lobby | `lobby_id`, `party_id` |
| `leave-lobby` | Leave lobby | `lobby_id`, `party_id` |
| `set-ready` | Mark party as ready | `lobby_id`, `party_id` |
| `unset-ready` | Mark party as not ready | `lobby_id`, `party_id` |
| `start-game` | Start game (host only) | `lobby_id` |

## Service Endpoints (Internal)

### From Matchmaking Shard

| Endpoint | Description |
|----------|-------------|
| `matchmaking.match` | Receive completed match |
| `matchmaking.backfill-match` | Receive backfill players |

### From Game Shard

| Endpoint | Description |
|----------|-------------|
| `backfill.request` | Request backfill players |
| `backfill.cancel` | Cancel backfill request |
| `game.heartbeat` | Keep lobby alive during game |
| `game.end-match` | Game finished, cleanup lobby |

### Query Endpoints

| Endpoint | Description |
|----------|-------------|
| `query.party` | Get party by ID |
| `query.party-by-player` | Get party for player |
| `query.lobby` | Get lobby by ID |
| `query.lobby-by-player` | Get lobby for player |
| `query.list-lobbies` | List joinable lobbies |
| `query.stats` | Get shard statistics |

## Game Shard Communication

| Event | Direction | When |
|-------|-----------|------|
| Player list | Lobby → Game | Lobby created / player joins |
| Start game | Lobby → Game | Ready state reached |
| Heartbeat | Game → Lobby | Every 5 min while in_game |
| Backfill request | Game → Lobby → Matchmaking | Player leaves mid-game |
| End match | Game → Lobby | Game finishes |

### Address Discovery

Game Shard knows Lobby address because:
1. `target_address` (Game Shard) is configured in MatchProfile
2. When Lobby sends "start game", it includes its own address
3. Game Shard stores this for callbacks

```json
// config/match_profiles.json
{
  "name": "1v1-ranked",
  "target_address": {
    "service_id": "game-1"
  },
  "lobby_address": {
    "service_id": "lobby-1"
  }
}
```

## Security Considerations

### Party ID Validation

**Never trust client-provided party IDs.** All party operations validate:
1. Party exists in PartyStore
2. Requesting player is a member (or leader for privileged ops)
3. Party state allows the operation

### Join Code Security

- Codes are random, not sequential
- Invalid code attempts are rate-limited
- Codes are single-use for invite flow
- Codes expire with party

### Matchmaking Gateway

Lobby acts as gateway to Matchmaking:
- Clients cannot directly create matchmaking tickets
- Lobby validates party ownership before forwarding
- Prevents party ID spoofing attacks

## Data Stores

### PartyStore

```go
// Primary storage
partiesByID map[string]*Party

// Index by player
partyByPlayer map[string]string

// Index by join code
partyByJoinCode map[string]string
```

### LobbyStore

```go
// Primary storage
lobbiesByID map[string]*Lobby

// Index by match
lobbyByMatch map[string]string

// Index by state
lobbiesByState map[LobbyState]map[string]bool

// Index by party
lobbyByParty map[string]string
```

## Usage Example

```go
package main

import (
    "github.com/argus-labs/world-engine/pkg/lobby"
    "github.com/argus-labs/world-engine/pkg/micro"
)

func main() {
    world, err := lobby.NewWorld(lobby.WorldOptions{
        Region:              "us-east",
        Organization:        "my-org",
        Project:             "my-game",
        ShardID:             "lobby-1",
        TickRate:            10,
        EpochFrequency:      100,
        SnapshotStorageType: micro.SnapshotStorageNop,
    })
    if err != nil {
        log.Fatal(err)
    }

    world.StartLobby()
}
```

## References

- [STATE.md](./STATE.md) - Lobby state machine details
- [Open Match](https://github.com/googleforgames/open-match) - Google's matchmaking framework
- [Steam Parties API](https://partner.steamgames.com/doc/api/isteamparties) - Steam's party system
- [Epic EOS Parties](https://dev.epicgames.com/docs/en-US/epic-account-services/social-overlay-overview/your-party) - Epic's implementation
