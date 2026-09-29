package system

import (
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/component"
)

// ChatRow is the archetype of chat message entities.
type ChatRow struct {
	UserTag component.UserTag
	Chat    component.Chat
}
