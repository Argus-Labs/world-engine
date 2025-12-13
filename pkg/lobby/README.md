# Lobby Package

A flexible lobby/party system for Cardinal worlds, handling player grouping and session management.

## Quick Start

```go
import "github.com/argus-labs/world-engine/pkg/lobby"

func main() {
    world, _ := cardinal.NewWorld(cardinal.WorldOptions{})

    lobby.Register(world, lobby.Config{
        // Cross-shard: notify game shard when session starts
        GameShardID:      "game-shard-1",
        GameRegion:       "us-west",
        GameOrganization: "myorg",
        GameProject:      "myproject",
        // Include lobby address so game shard can send EndSession back
        LobbyShardAddress: "us-west.world.myorg.myproject.lobby-shard-1",
    })

    world.StartGame()
}
```

## Custom Provider

Override invite code generation:

```go
type MyProvider struct {
    lobby.DefaultProvider
}

func (p MyProvider) GenerateInviteCode(l *lobby.LobbyComponent) string {
    return generateMyCustomCode(8)
}

lobby.Register(world, lobby.Config{
    GameShardID: "game-shard-1",
    Provider:    MyProvider{},
})
```

Default: `Hash(LobbyID + Timestamp)` -> 6-char uppercase alphanumeric (excludes confusing chars: 0, O, I, L, 1).

## Overview

- **PvE**: Lobby acts as a persistent party that plays games together
- **PvP**: Lobby acts as a temporary session where multiple teams gather

## Data Structures

### Lobby

```
Lobby
- ID string              // Unique identifier
- LeaderID string        // Player who controls the lobby
- Teams []Team           // List of teams
- InviteCode string      // Code for others to join
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
```

### Session

```
Session
- State SessionState             // idle | in_session
- PassthroughData map[string]any // Forwarded to game shard
```

## Configuration

| Option | Description |
|--------|-------------|
| `GameShardID` | Game shard ID to notify when session starts |
| `GameRegion` | Game shard region (required if GameShardID set) |
| `GameOrganization` | Game shard organization (required if GameShardID set) |
| `GameProject` | Game shard project (required if GameShardID set) |
| `LobbyShardAddress` | This lobby's full address (for EndSession callback) |
| `Provider` | Custom provider (optional, default provided) |

## Commands

| Command | Description |
|---------|-------------|
| `CreateLobby` | Create lobby, become leader |
| `JoinLobby(inviteCode, teamID?)` | Join via invite code |
| `JoinTeam(teamID)` | Move to different team |
| `LeaveLobby` | Leave lobby |
| `SetReady(isReady)` | Set ready status |
| `KickPlayer(targetPlayerID)` | Leader kicks player |
| `TransferLeader(targetPlayerID)` | Transfer leadership |
| `StartSession(passthroughData?)` | Start session |
| `EndSession(lobbyID)` | End session |
| `GenerateInviteCode` | Generate new code |

## Cross-Shard Communication

When `StartSession` is called, `NotifySessionStartCommand` is sent to game shard:

```go
type NotifySessionStartCommand struct {
    Lobby             LobbyComponent // Entire lobby
    LobbyShardAddress string         // For EndSession callback
}
```

Game shard should register a handler for this command and send `EndSessionCommand` back when game ends.

## Queries

Query lobby data using Cardinal's generic search API:

```go
// Find all lobbies
results, _ := world.NewSearch(ecs.SearchParam{
    Find:  []string{"lobby"},
    Match: "contains",
})

// Find lobby by invite code
results, _ := world.NewSearch(ecs.SearchParam{
    Find:  []string{"lobby"},
    Match: "contains",
    Where: `lobby.invite_code == "ABC123"`,
})
```

## Lifecycle

```
CreateLobby -> idle -> (players join, ready up) -> StartSession -> in_session -> EndSession -> idle
```

Lobby persists after session ends. Players can start another session.
