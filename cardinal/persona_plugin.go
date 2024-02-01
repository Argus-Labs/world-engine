package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/persona/query"
)

type PersonaPlugin struct {
}

func New() *PersonaPlugin {
	return &PersonaPlugin{}
}

func (p *PersonaPlugin) Register(world *World) error {
	err := p.RegisterQueries(world)
	if err != nil {
		return err
	}
	err = p.RegisterSystems(world)
	if err != nil {
		return err
	}
	err = p.RegisterComponents(world)
	if err != nil {
		return err
	}
	err = p.RegisterMessages(world)
	if err != nil {
		return err
	}
	return nil
}

func (p *PersonaPlugin) RegisterQueries(world *World) error {
	err := RegisterQuery[query.PersonaSignerQueryRequest, query.PersonaSignerQueryResponse](world, "signer",
		query.PersonaSignerQuery,
		WithCustomQueryGroup[query.PersonaSignerQueryRequest, query.PersonaSignerQueryResponse]("persona"))
	if err != nil {
		return err
	}
	return nil
}

func (p *PersonaPlugin) RegisterSystems(world *World) error {
	err := RegisterSystems(world, RegisterPersonaSystem, AuthorizePersonaAddressSystem)
	if err != nil {
		return err
	}
	return nil
}

func (p *PersonaPlugin) RegisterComponents(world *World) error {
	err := RegisterComponent[component.SignerComponent](world)
	if err != nil {
		return err
	}
	return nil
}

func (p *PersonaPlugin) RegisterMessages(world *World) error {
	err := RegisterMessages(world, CreatePersonaMsg, AuthorizePersonaAddressMsg)
	if err != nil {
		return err
	}
	return nil
}

var AuthorizePersonaAddressMsg = NewMessageType[msg.AuthorizePersonaAddress, msg.AuthorizePersonaAddressResult](
	"authorize-persona-address",
)

// CreatePersonaMsg is a message that facilitates the creation of a persona tag.
var CreatePersonaMsg = NewMessageType[msg.CreatePersona, msg.CreatePersonaResult](
	"create-persona",
	WithCustomMessageGroup[msg.CreatePersona, msg.CreatePersonaResult]("persona"),
	WithMsgEVMSupport[msg.CreatePersona, msg.CreatePersonaResult](),
)
