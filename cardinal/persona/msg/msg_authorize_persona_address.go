package msg

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type AuthorizePersonaAddress struct {
	Address string `json:"address"`
}

type AuthorizePersonaAddressResult struct {
	Success bool `json:"success"`
}

var AuthorizePersonaAddressMsg = ecs.NewMessageType[AuthorizePersonaAddress, AuthorizePersonaAddressResult](
	"authorize-persona-address",
)
