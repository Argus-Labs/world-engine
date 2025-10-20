package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/examples/chat/component"
)

type ChatSearch = cardinal.Exact[struct {
	UserTag cardinal.Ref[component.UserTag]
	Chat    cardinal.Ref[component.Chat]
}]
