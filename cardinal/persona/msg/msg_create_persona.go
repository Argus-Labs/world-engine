package msg

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
)

// CreatePersona allows for the associating of a persona tag with a signer address.
type CreatePersona struct {
	PersonaTag    string `json:"personaTag"`
	SignerAddress string `json:"signerAddress"`
}

type CreatePersonaResult struct {
	Success bool `json:"success"`
}

// CreatePersonaMsg is a message that facilitates the creation of a persona tag.
var CreatePersonaMsg = ecs.NewMessageType[CreatePersona, CreatePersonaResult](
	"create-persona",
	ecs.WithCustomMessageGroup[CreatePersona, CreatePersonaResult]("persona"),
	ecs.WithMsgEVMSupport[CreatePersona, CreatePersonaResult](),
)
