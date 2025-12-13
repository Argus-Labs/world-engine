# Lobby Package

A matchmaking lobby system for Cardinal worlds, handling game lifecycle for matches created by the matchmaking package.

> **NOTICE: Matchmaking Lobbies Only**
>
> This package handles **matchmaking-created lobbies only**. When matchmaking finds a match, it sends a `CreateLobbyFromMatch` command/event to create a lobby that immediately transitions to `in_game` state.
>
> Future improvement: Manual lobby features (create, join, leave, ready-up, kick, lobby browser).

> **NOTICE: Party Management**
>
> Parties are **not** managed by the lobby shard. The `party_id` field on tickets and lobbies is simply a grouping identifier - there is no party entity or lifecycle management in this package.
>
> Party management should live in a dedicated **Social Shard** that handles:
>
> - **Party Management** - Create/join/leave party, invites, leader promotion, kick members
> - **Friends System** - Friend requests, friend list, block list, online/offline status
> - **Chat** - Party chat, whispers/DMs, lobby chat
> - **Presence** - Player online status, activity status ("In Game", "In Queue", "In Lobby"), last seen

## Architecture Overview

This package implements an ECS-based lobby system for matchmaking-created games.

```text
World
├── LobbyIndexComponent (singleton)
│   ├── MatchIDToEntity (map[string]uint32)
│   └── InGameLobbies ([]string)
│
├── ConfigComponent (singleton)
│   ├── HeartbeatTimeoutSeconds
│   ├── MatchmakingShardID
│   └── GameShardID
│
├── Lobbies ([]LobbyComponent)
│   ├── MatchID, Parties
│   ├── Teams []LobbyTeam
│   ├── State (in_game/ended)
│   ├── DisconnectedParties
│   └── Timestamps
│
└── Systems
    ├── InitSystem (Init hook) - creates singletons
    └── LobbySystem (Update hook) - processes commands, heartbeat timeout
```

### Core Components

1. **LobbyComponent**: Game session container with state machine (in_game → ended)
2. **LobbyIndexComponent**: Singleton for O(1) lobby lookups by match ID

### Key Design Features

1. **Matchmaking Integration**: Receives matches from matchmaking, creates lobbies automatically
2. **Simple State Machine**: Lobbies are created in `in_game` state, transition to `ended` when game completes
3. **Cross-Shard Support**: Receives matches from matchmaking shard, notifies game shard on start
4. **Disconnect Tracking**: Tracks disconnected parties for backfill decisions
5. **Heartbeat Timeout**: Stale lobbies are cleaned up automatically
6. **Extensible**: Users can add custom systems alongside lobby

## Installation

```bash
go get github.com/argus-labs/world-engine/pkg/lobby
```

## Usage

```go
package main

import (
    "github.com/argus-labs/world-engine/pkg/cardinal"
    "github.com/argus-labs/world-engine/pkg/lobby"
)

func main() {
    world, _ := cardinal.NewWorld(cardinal.WorldOptions{
        TickRate: 10,
    })

    lobby.Register(world, lobby.Config{
        HeartbeatTimeoutSeconds: 60,
        MatchmakingShardID:      "matchmaking-1", // empty for same-shard
        GameShardID:             "game-1",        // empty for same-shard
        GameRegion:              "local",
        GameOrganization:        "demo",
        GameProject:             "my-game",
    })

    world.StartGame()
}
```

## API Reference

### Commands Received (Inputs)

Commands that lobby shard receives from other shards or clients:

| Command | From | Description |
|---------|------|-------------|
| `matchmaking_create_lobby_from_match` | Matchmaking Shard | Create lobby from match result |
| `game_notify_lobby_end` | Game Shard | Game ended, close lobby |
| `game_player_disconnected` | Game Shard | Player disconnected (state tracking) |
| `lobby_end_game` | Client / Game | End game with results |
| `lobby_heartbeat` | Client / Game | Keep lobby alive during gameplay |

### Commands Sent (Outputs)

Commands that lobby shard sends to other shards:

| Command | To | Description |
|---------|-----|-------------|
| `lobby_notify_game_start` | Game Shard | Notify game to start with match/team info |

### Events Emitted (Outputs)

Events emitted to clients subscribed to lobby shard:

| Event | Description |
|-------|-------------|
| `lobby_created` | Lobby created from match |
| `lobby_game_started` | Game started (emitted immediately after creation) |
| `lobby_game_ended` | Game ended |
| `lobby_player_disconnected` | Player marked as disconnected |
| `lobby_error` | Operation failed |

## Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `HeartbeatTimeoutSeconds` | int64 | 60 | Lobby timeout without heartbeat |
| `MatchmakingShardID` | string | "" | Source matchmaking shard (empty for same-shard) |
| `GameShardID` | string | "" | Target game shard (empty for same-shard) |
| `GameRegion` | string | "" | Region for cross-shard game communication |
| `GameOrganization` | string | "" | Organization for cross-shard game communication |
| `GameProject` | string | "" | Project for cross-shard game communication |

## Extending with Custom Systems

You can add custom systems alongside the lobby package using `cardinal.RegisterSystem`. Custom systems can query lobby entities and react to lobby state changes.

```go
package main

import (
    "github.com/argus-labs/world-engine/pkg/cardinal"
    "github.com/argus-labs/world-engine/pkg/lobby"
    lobbyComponent "github.com/argus-labs/world-engine/pkg/lobby/component"
)

func main() {
    world, _ := cardinal.NewWorld(cardinal.WorldOptions{TickRate: 10})

    // Register lobby package first
    lobby.Register(world, lobby.Config{
        HeartbeatTimeoutSeconds: 60,
    })

    // Register custom systems after lobby
    cardinal.RegisterSystem(world, MyCustomInitSystem, cardinal.WithHook(cardinal.Init))
    cardinal.RegisterSystem(world, MyCustomSystem)

    world.StartGame()
}

// Custom system state - can query lobby entities
type MyCustomSystemState struct {
    cardinal.BaseSystemState

    // Query lobby entities created by the lobby package
    Lobbies cardinal.Contains[struct {
        Lobby cardinal.Ref[lobbyComponent.LobbyComponent]
    }]
}

// Custom system runs every tick alongside lobby system
func MyCustomSystem(state *MyCustomSystemState) error {
    // Count active lobbies
    activeCount := 0
    for _, lobbyEntity := range state.Lobbies.Iter() {
        lobby := lobbyEntity.Lobby.Get()
        if lobby.State == lobbyComponent.LobbyStateInGame {
            activeCount++
        }
    }

    // Log periodically
    if state.Tick()%100 == 0 {
        state.Logger().Info().Int("active_lobbies", activeCount).Msg("Lobby stats")
    }

    return nil
}
```

## Lobby Lifecycle

```
Matchmaking finds match
        │
        ▼
┌───────────────────┐
│ CreateLobbyFromMatch │
│ (from matchmaking)   │
└─────────┬─────────┘
          │
          ▼
┌───────────────────┐
│     in_game       │ ◄─── Lobby created, game starts immediately
│                   │      NotifyGameStart sent to game shard
└─────────┬─────────┘
          │
          │ EndGameCommand or
          │ NotifyGameEndCommand or
          │ HeartbeatTimeout
          │
          ▼
┌───────────────────┐
│      ended        │ ◄─── Lobby destroyed
└───────────────────┘
```

## Contributing

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for development setup and guidelines.

## License

See [LICENSE](../../LICENSE) for details.
