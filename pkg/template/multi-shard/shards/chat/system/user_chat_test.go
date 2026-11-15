package system_test

import (
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/event"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserChatUsesTickTimestamp(t *testing.T) {
	t.Parallel()

	w := cardinal.NewTestWorld(t, registerChat)
	w.Tick() // Run the chat tick at a nonzero height.

	// A time unrelated to both the tick height and the wall clock.
	timestamp := time.Date(2001, time.February, 3, 4, 5, 6, 0, time.UTC)

	w.Command("alice", command.UserChat{ArgusAuthID: "alice", ArgusAuthName: "Alice", Message: "hello"})
	w.World.Tick(timestamp)

	chat, err := w.Contains[system.ChatRow]().Iter().Single()
	require.NoError(t, err)

	stored := chat.Get[component.Chat]()
	stored.Timestamp = stored.Timestamp.UTC()
	assert.Equal(t, component.Chat{Message: "hello", Timestamp: timestamp}, stored)

	sent := w.Events[event.UserChat]()
	require.Len(t, sent, 1)

	sent[0].Payload.Timestamp = sent[0].Payload.Timestamp.UTC()
	assert.Equal(t, event.UserChat{
		ArgusAuthID: "alice", ArgusAuthName: "Alice", Message: "hello", Timestamp: timestamp,
	}, sent[0].Payload)
}

func TestUserChatDeterministic(t *testing.T) {
	t.Parallel()

	cardinal.RequireDeterministic(t, registerChat, func(w *cardinal.TestWorld) {
		w.Command("alice", command.UserChat{ArgusAuthID: "alice", ArgusAuthName: "Alice", Message: "hello"})
		w.Tick()
	})
}

func registerChat(w *cardinal.World) {
	w.RegisterComponent[component.UserTag]()
	w.RegisterComponent[component.Chat]()
	w.RegisterCommand[command.UserChat]()
	w.RegisterEvent[event.UserChat]()
	w.RegisterSystem(&system.UserChatSystem{})
}
