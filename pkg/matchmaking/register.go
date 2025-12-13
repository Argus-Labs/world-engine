// Package matchmaking provides an embeddable matchmaking system for Cardinal worlds.
//
// Usage:
//
//	world := cardinal.NewWorld(cardinal.WorldOptions{...})
//	matchmaking.Register(world, matchmaking.Config{
//		DefaultTTLSeconds:  300, // 5 minutes
//		BackfillTTLSeconds: 60,  // 1 minute
//	})
//	world.StartGame()
//
// The package registers the following systems:
//   - InitSystem (Init hook): Creates singleton index entities
//   - MatchmakingSystem (Update hook): Processes tickets, runs matching, expires old entries
//
// The package provides the following commands:
//   - CreateTicketCommand: Create a matchmaking ticket
//   - CancelTicketCommand: Cancel an existing ticket
//   - CreateBackfillCommand: Request backfill for an ongoing match
//   - CancelBackfillCommand: Cancel a backfill request
//
// The package emits the following events:
//   - TicketCreatedEvent: Ticket was successfully created
//   - TicketCancelledEvent: Ticket was cancelled
//   - TicketErrorEvent: Ticket creation failed (e.g., duplicate player)
//   - MatchFoundEvent: A match was found
//   - BackfillMatchEvent: Backfill tickets were assigned
package matchmaking

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/matchmaking/component"
	"github.com/argus-labs/world-engine/pkg/matchmaking/system"
)

// Re-export types for easier user access
type (
	// Commands
	CreateTicketCommand   = system.CreateTicketCommand
	CancelTicketCommand   = system.CancelTicketCommand
	CreateBackfillCommand = system.CreateBackfillCommand
	CancelBackfillCommand = system.CancelBackfillCommand
	GetTicketsCommand     = system.GetTicketsCommand

	// Response Commands (cross-shard)
	TicketsListResponse = system.TicketsListResponse
	TicketInfo          = system.TicketInfo

	// Events (client-facing)
	TicketCreatedEvent   = system.TicketCreatedEvent
	TicketCancelledEvent = system.TicketCancelledEvent
	TicketErrorEvent     = system.TicketErrorEvent
	MatchFoundEvent      = system.MatchFoundEvent
	BackfillMatchEvent   = system.BackfillMatchEvent
	MatchTeam            = system.MatchTeam

	// System Events (for same-shard communication)
	CreateLobbyFromMatchEvent = system.CreateLobbyFromMatchEvent
	LobbyTeamInfo             = system.LobbyTeamInfo

	// Cross-shard Commands (for cross-shard communication)
	CreateLobbyFromMatchCommand = system.CreateLobbyFromMatchCommand

	// Components
	PlayerInfo       = component.PlayerInfo
	TicketComponent  = component.TicketComponent
	ProfileComponent = component.ProfileComponent
	PoolConfig       = component.PoolConfig
	TeamConfig       = component.TeamConfig
)

// Config holds configuration for the matchmaking package.
type Config struct {
	// DefaultTTLSeconds is the default ticket TTL in seconds.
	// If 0, defaults to 300 (5 minutes).
	DefaultTTLSeconds int64

	// BackfillTTLSeconds is the default backfill request TTL in seconds.
	// If 0, defaults to 60 (1 minute).
	BackfillTTLSeconds int64

	// LobbyShardID is the shard ID for cross-shard lobby communication.
	// If empty, same-shard communication via SystemEvents is used.
	LobbyShardID string

	// LobbyRegion is the region for the lobby shard (for cross-shard).
	// Required when LobbyShardID is set.
	LobbyRegion string

	// LobbyOrganization is the organization for the lobby shard (for cross-shard).
	// Required when LobbyShardID is set.
	LobbyOrganization string

	// LobbyProject is the project for the lobby shard (for cross-shard).
	// Required when LobbyShardID is set.
	LobbyProject string
}

// Register registers the matchmaking systems with the given world.
// This should be called before world.StartGame().
func Register(world *cardinal.World, config Config) {
	// Apply defaults
	if config.DefaultTTLSeconds <= 0 {
		config.DefaultTTLSeconds = 300
	}
	if config.BackfillTTLSeconds <= 0 {
		config.BackfillTTLSeconds = 60
	}

	// Store config for init system to use
	system.SetConfig(component.ConfigComponent{
		LobbyShardID:       config.LobbyShardID,
		LobbyRegion:        config.LobbyRegion,
		LobbyOrganization:  config.LobbyOrganization,
		LobbyProject:       config.LobbyProject,
		DefaultTTLSeconds:  config.DefaultTTLSeconds,
		BackfillTTLSeconds: config.BackfillTTLSeconds,
	})

	// Register init system (runs once during world initialization)
	cardinal.RegisterSystem(world, system.InitSystem, cardinal.WithHook(cardinal.Init))

	// Register main matchmaking system (runs every tick)
	cardinal.RegisterSystem(world, system.MatchmakingSystem)

	// Register GetTickets system (handles ticket list queries)
	cardinal.RegisterSystem(world, system.GetTicketsSystem)
}
