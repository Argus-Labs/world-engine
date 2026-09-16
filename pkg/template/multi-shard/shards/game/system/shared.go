package system

import (
	"sync"

	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
)

// Player is the archetype of player entities.
type Player struct {
	Tag      component.PlayerTag
	Position component.Position
	Online   component.OnlineStatus
}

// PlayerSet manages a thread-safe set of player IDs.
type PlayerSet struct {
	mu      sync.RWMutex
	players map[string]bool
}

// NewPlayerSet creates a new PlayerSet.
func NewPlayerSet() *PlayerSet {
	return &PlayerSet{
		players: make(map[string]bool),
	}
}

// Exists checks if a player exists in the set.
func (p *PlayerSet) Exists(argusAuthID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.players[argusAuthID]
}

// Add adds a player to the set.
func (p *PlayerSet) Add(argusAuthID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.players[argusAuthID] = true
}

// Clear clears the player set.
func (p *PlayerSet) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.players = make(map[string]bool)
}

// Global player set instance.
var playerSet = NewPlayerSet() //nolint:gochecknoglobals // this set is only used for optimization purposes
