package command_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/stretchr/testify/assert"
)

func TestSender(t *testing.T) {
	t.Parallel()

	player := command.PlayerSender("alice")
	id, ok := player.Player()
	assert.Equal(t, "alice", id)
	assert.True(t, ok)
	_, ok = player.Shard()
	assert.False(t, ok)
	assert.Equal(t, "alice", player.ID())

	shard := command.ShardSender(&microv1.ServiceAddress{
		Region:       "us",
		Realm:        microv1.ServiceAddress_REALM_WORLD,
		Organization: "org",
		Project:      "proj",
		ServiceId:    "lobby",
	})
	address, ok := shard.Shard()
	assert.Equal(t, "us.world.org.proj.lobby", address)
	assert.True(t, ok)
	_, ok = shard.Player()
	assert.False(t, ok)
	assert.Equal(t, "us.world.org.proj.lobby", shard.ID())

	_, ok = command.Sender{}.Player()
	assert.False(t, ok, "a zero sender is not a player")
}
