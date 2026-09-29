package system

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/argus-labs/world-engine/pkg/plugin/lobby/component"
)

// Config is the runtime configuration the systems read, supplied by the shard's main.go through
// the lobby plugin at registration.
//
// It lives here rather than in the component package because it is not a component: it has no
// Name(), is wired to no system, never becomes an entity, and is never snapshotted. The component
// package holds only types that are actually stored in the world.
type Config struct {
	// LobbyWorld is this lobby shard's address (for game shard to send NotifySessionEndCommand back).
	LobbyWorld component.ShardAddress `json:"lobby_world"`

	// HeartbeatTimeout is how long (in seconds) before a player is removed for not sending heartbeats.
	// Clients should send heartbeats more frequently than this (e.g., every timeout/3 seconds).
	// Default: 30 seconds.
	HeartbeatTimeout int64 `json:"heartbeat_timeout"`

	// AssignmentAuthority is an accident-prevention filter, NOT an
	// authentication boundary. The plugin compares it against cmd.Persona
	// and drops mismatches. This prevents an unrelated system that
	// happens to send AssignShardCommand from accidentally completing the
	// wrong lobby's session start. It does NOT defend against a client
	// that forges Persona, because cmd.Persona is not signature-verified
	// at this layer. Real authentication must live above the plugin
	// (NATS ACLs, gateway auth, signed commands). Empty = no filter.
	AssignmentAuthority string `json:"assignment_authority,omitempty"`

	// MaxAllocationTimeout bounds how long (in seconds) a lobby may remain
	// in SessionStateAwaitingAllocation before the lobby shard fails the
	// start itself and returns to Idle. Values <= 0 disable timeout
	// enforcement entirely.
	MaxAllocationTimeout int64 `json:"max_allocation_timeout,omitempty"`
}

// Runtime owns one world's deployment configuration and derived lobby lookups.
// Configuration stays outside ECS so restored snapshots use the running deployment's
// settings. Each plugin instance creates its own runtime and shares it only among its systems.
type Runtime struct {
	config     Config
	provider   LobbyProvider
	presets    map[string][]component.TeamConfig
	index      lookupIndex
	indexBuilt bool
}

// NewRuntime validates and copies the server-owned configuration.
// Invalid presets panic at registration rather than rejecting every lobby creation at runtime.
func NewRuntime(config Config, provider LobbyProvider, presets map[string][]component.TeamConfig) *Runtime {
	// Every bad preset at once, sorted: map order is random, so reporting the first one found would
	// make an operator with two mistakes fix one, redeploy, and hit the other.
	var unusable []string
	for name, teams := range presets {
		if reason := validatePreset(teams); reason != "" {
			unusable = append(unusable, fmt.Sprintf("%q: %s", name, reason))
		}
	}
	if len(unusable) > 0 {
		sort.Strings(unusable)
		panic("unusable lobby presets:\n  " + strings.Join(unusable, "\n  "))
	}

	if config.HeartbeatTimeout <= 0 {
		config.HeartbeatTimeout = 30
	}
	if provider == nil {
		provider = DefaultProvider{}
	}
	ownedPresets := make(map[string][]component.TeamConfig, len(presets))
	for name, teams := range presets {
		ownedPresets[name] = slices.Clone(teams)
	}
	return &Runtime{config: config, provider: provider, presets: ownedPresets}
}
