package system

import (
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/component"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type ChatSearch = cardinal.Exact[struct {
	UserTag cardinal.Ref[component.UserTag]
	Chat    cardinal.Ref[component.Chat]
}]
