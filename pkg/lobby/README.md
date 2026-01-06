# Lobby Package

A flexible lobby/party system for Cardinal worlds, handling player grouping and session management.

## Quick Start

```go
import "github.com/argus-labs/world-engine/pkg/lobby"

func main() {
    world, _ := cardinal.NewWorld(cardinal.WorldOptions{})

    lobby.Register(world, lobby.Config{
        // LobbyWorld is this lobby shard's address (so game shard can send back NotifySessionEndCommand)
        LobbyWorld: cardinal.OtherWorld{
            Region:       "us-west",
            Organization: "myorg",
            Project:      "myproject",
            ShardID:      "lobby-shard-1",
        },
    })

    world.StartGame()
}
```

## Overview

- **PvE**: Lobby acts as a persistent party that plays games together
- **PvP**: Lobby acts as a temporary session where multiple teams gather

## Message Categories

The lobby package uses 4 message categories:

### 1. Command (Client ↔ Shard)
Client sends a command to the shard. Shard processes it and returns a result to the sender.

| Command | Request Fields | Response Fields |
|---------|----------------|-----------------|
| `CreateLobby` | `Teams []TeamConfig`, `GameWorld`, `PlayerPassthroughData?`, `SessionPassthroughData?` | `IsSuccess`, `Message`, `Lobby` |
| `JoinLobby` | `InviteCode`, `TeamName?`, `PlayerPassthroughData?` | `IsSuccess`, `Message`, `Lobby` |
| `JoinTeam` | `TeamName` | `IsSuccess`, `Message` |
| `LeaveLobby` | - | `IsSuccess`, `Message` |
| `SetReady` | `IsReady` | `IsSuccess`, `Message` |
| `KickPlayer` | `TargetPlayerID` | `IsSuccess`, `Message` |
| `TransferLeader` | `TargetPlayerID` | `IsSuccess`, `Message` |
| `StartSession` | - | `IsSuccess`, `Message` |
| `GenerateInviteCode` | - | `IsSuccess`, `Message`, `InviteCode` |
| `UpdateSessionPassthrough` | `PassthroughData` | `IsSuccess`, `Message` |
| `UpdatePlayerPassthrough` | `PassthroughData` | `IsSuccess`, `Message` |
| `Heartbeat` | - | - (no response) |

All commands include `RequestID` for request/response correlation (except `Heartbeat`).

### 2. Event (Shard → All Clients, broadcast)
Shard broadcasts state changes to all subscribed clients.

| Event | Fields | Description |
|-------|--------|-------------|
| `LobbyCreatedEvent` | `LobbyID`, `LeaderID`, `InviteCode` | Lobby created |
| `PlayerJoinedEvent` | `LobbyID`, `TeamName`, `Player` (PlayerState) | Player joined |
| `PlayerLeftEvent` | `LobbyID`, `PlayerID` | Player left |
| `PlayerKickedEvent` | `LobbyID`, `PlayerID`, `KickerID` | Player kicked |
| `PlayerReadyEvent` | `LobbyID`, `PlayerID`, `IsReady` | Ready status changed |
| `PlayerChangedTeamEvent` | `LobbyID`, `PlayerID`, `OldTeamName`, `NewTeamName` | Player changed team |
| `LeaderChangedEvent` | `LobbyID`, `OldLeaderID`, `NewLeaderID` | Leadership transferred |
| `SessionStartedEvent` | `LobbyID` | Session started |
| `SessionEndedEvent` | `LobbyID` | Session ended |
| `InviteCodeGeneratedEvent` | `LobbyID`, `InviteCode` | New code generated |
| `LobbyDeletedEvent` | `LobbyID` | Lobby deleted (empty) |
| `PlayerTimedOutEvent` | `LobbyID`, `PlayerID` | Player removed due to missed heartbeats |
| `SessionPassthroughUpdatedEvent` | `Lobby` | Session passthrough data updated |
| `PlayerPassthroughUpdatedEvent` | `Lobby`, `PlayerID` | Player passthrough data updated |

### 3. CrossShardCommand (Shard → Shard)
One shard sends a command to another shard.

| Command | Direction | Fields | Description |
|---------|-----------|--------|-------------|
| `NotifySessionStartCommand` | Lobby → Game | `Lobby`, `LobbyWorld` | Notify game shard to start session |
| `NotifySessionEndCommand` | Game → Lobby | `LobbyID` | Notify lobby shard to end session |

### 4. CrossShardResult (Shard → Shard, response)
Shard sends a response back to the originating shard.

*Currently not needed. CrossShardCommands in this package don't require responses.*

## Data Structures

### Lobby

```
Lobby
- ID string              // Unique identifier
- LeaderID string        // Player who controls the lobby
- Teams []Team           // List of teams
- InviteCode string      // Code for others to join
- GameWorld OtherWorld   // Target game shard address
- Session Session        // Current session state
- CreatedAt int64        // Unix timestamp
```

### Team

```
Team
- TeamID string          // Unique team identifier
- Name string            // Team display name
- Players []PlayerState  // Players in team
- MaxPlayers int         // Max players (0 = unlimited)
```

### PlayerState

```
PlayerState
- PlayerID string                 // Player identifier
- IsReady bool                    // Ready status
- PassthroughData map[string]any  // Forwarded to game shard
- JoinedAt int64                  // Unix timestamp when player joined
```

### Session

```
Session
- State SessionState             // idle | in_session
- PassthroughData map[string]any // Forwarded to game shard
```

## Configuration

| Option | Description | Default |
|--------|-------------|---------|
| `LobbyWorld` | This lobby shard's address (for game shard to send NotifySessionEndCommand back) | required |
| `Provider` | Custom provider (optional, default provided) | `DefaultProvider` |
| `HeartbeatInterval` | How often clients should send heartbeats (seconds) | 10 |
| `HeartbeatMaxMisses` | Consecutive missed heartbeats before player removal | 3 |

## Cross-Shard Communication

When `StartSessionCommand` succeeds, `NotifySessionStartCommand` is sent to game shard:

```go
type NotifySessionStartCommand struct {
    Lobby      LobbyComponent    // Full lobby data (includes GameWorld)
    LobbyWorld cardinal.OtherWorld // For NotifySessionEndCommand callback
}
```

Game shard should:
1. Register a system that handles `NotifySessionStartCommand`
2. Run the game
3. Send `NotifySessionEndCommand` back to lobby when game ends

```go
// In game shard
func GameSessionSystem(state *GameSessionSystemState) error {
    for cmd := range state.SessionStartCmds.Iter() {
        payload := cmd.Payload()
        // payload.Lobby contains full lobby data
        // payload.LobbyWorld for sending NotifySessionEndCommand back

        // When game ends:
        payload.LobbyWorld.Send(&state.BaseSystemState, NotifySessionEndCommand{
            LobbyID: payload.Lobby.ID,
        })
    }
    return nil
}
```

## Custom Provider

Override invite code generation:

```go
type MyProvider struct {
    lobby.DefaultProvider
}

func (p MyProvider) GenerateInviteCode(l *component.LobbyComponent) string {
    return generateMyCustomCode(8)
}

lobby.Register(world, lobby.Config{
    LobbyWorld: cardinal.OtherWorld{...},
    Provider:   MyProvider{},
})
```

Default: `Hash(LobbyID + Timestamp)` -> 6-char uppercase alphanumeric (excludes confusing chars: 0, O, I, L, 1).

## Lifecycle

```
CreateLobby -> idle -> (players join, ready up) -> StartSession -> in_session -> EndSession -> idle
                                                                                      │
                                                                                      v
                                                                        (players can start again)
```

## Key Behaviors

- **Invite code persists** on leader transfer/leave (no auto-regeneration)
- **Leader-only actions**: kick, transfer, start session, regenerate code
- **Ready check**: All players (including leader) must be ready to start
- **Session lock**: No join/team-change during `in_session` (leave is always allowed)
- **Auto-reset**: After session ends, all players reset to not-ready
- **Auto-delete**: Lobby deleted when last player leaves
- **Per-lobby game shard**: Each lobby specifies its target game shard via `GameWorld`

## Heartbeat System

The lobby package includes a heartbeat mechanism to detect and remove disconnected players. It uses a **deadline-based (lease) approach** where clients must continuously renew their presence.

### How It Works

1. **Clients send `HeartbeatCommand`** periodically (e.g., every `HeartbeatInterval` seconds)
2. **Server stores a deadline** for each player: `deadline = now + timeout`
3. **On each heartbeat**, the deadline is extended: `deadline = now + timeout`
4. **Server removes players** when `now >= deadline` (player failed to renew their lease)
5. **Events emitted**: `PlayerTimedOutEvent` and `PlayerLeftEvent` when a player times out

### Deadline Approach

The timeout is calculated as:
```
timeout = HeartbeatInterval × HeartbeatMaxMisses
```

When a player joins or sends a heartbeat:
```
deadline = currentTime + timeout
```

When checking for timeouts (every tick):
```
if currentTime >= deadline {
    // Player timed out - remove from lobby
}
```

This approach is simpler than tracking "last heartbeat time" because:
- No subtraction needed at check time (`now >= deadline` vs `now - lastHeartbeat > timeout`)
- The deadline is the actual removal time, making debugging easier

### Configuration

```go
lobby.Register(world, lobby.Config{
    LobbyWorld:         lobby.GameWorld{...},
    HeartbeatInterval:  10, // Clients should send heartbeat every 10 seconds
    HeartbeatMaxMisses: 3,  // Remove after 3 misses (30 second timeout)
})
```

### Client Implementation

Clients should:
1. Start a heartbeat timer **immediately after creating or joining a lobby**
2. Send `HeartbeatCommand` at the configured interval
3. Stop the timer after leaving the lobby

```go
// Client-side pseudocode
ticker := time.NewTicker(10 * time.Second)
go func() {
    for range ticker.C {
        sendCommand(lobby.HeartbeatCommand{})
    }
}()
```

**Important**: Start heartbeats immediately after joining. The timeout clock starts the moment a player joins, not when they send their first heartbeat.

### Timeout Behavior

When a player joins a lobby, their deadline is set to `now + timeout`. This means:
- Players must send heartbeats before their deadline expires or they are removed
- This prevents "zombie" players who join but never send heartbeats from occupying slots indefinitely

When a player times out:
- Player is removed from the lobby
- `PlayerTimedOutEvent` is emitted (for timeout-specific handling)
- `PlayerLeftEvent` is emitted (for consistency with normal leave)
- If player was leader, leadership auto-transfers to another player
- If lobby becomes empty, it is deleted
