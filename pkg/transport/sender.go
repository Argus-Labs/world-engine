package transport

import (
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/micro"
)

// Sender identifies who sent a command: a player or another shard. The transport builds one for every
// command it receives, with PlayerSender or ShardSender, so a received command's sender is always one
// of the two.
type Sender struct {
	id    string // Player ID, or the sending shard's address
	shard bool   // Whether id is a shard's address
}

// PlayerSender is the sender of a command a client sent as the authenticated player id.
func PlayerSender(id string) Sender {
	assert.That(id != "", "player sender has empty ID")
	return Sender{id: id}
}

// ShardSender is the sender of a command the service at address sent.
func ShardSender(address *micro.ServiceAddress) Sender {
	assert.That(address != nil, "shard sender has nil address")
	return Sender{id: micro.String(address), shard: true}
}

// ID returns the player ID, or the sending shard's address ("region.realm.org.project.shard").
// Use it when any sender will do, such as in logs.
func (s Sender) ID() string { return s.id }

// Player returns the player ID and true when a player sent the command.
func (s Sender) Player() (string, bool) {
	if s.shard {
		return "", false
	}
	return s.id, s.id != ""
}

// Shard returns the sending shard's address and true when another shard sent the command.
func (s Sender) Shard() (string, bool) {
	if !s.shard {
		return "", false
	}
	return s.id, true
}
