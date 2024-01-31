package persona

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/persona/query"
	"pkg.world.dev/world-engine/cardinal/persona/system"
)

type InternalPlugin struct {
}

var _ ecs.InternalPlugin = (*InternalPlugin)(nil)

func NewInternalPlugin() *InternalPlugin {
	return &InternalPlugin{}
}

func (p *InternalPlugin) Register(engine *ecs.Engine) error {
	err := p.RegisterQueries(engine)
	if err != nil {
		return err
	}
	err = p.RegisterSystems(engine)
	if err != nil {
		return err
	}
	err = p.RegisterComponents(engine)
	if err != nil {
		return err
	}
	err = p.RegisterMessages(engine)
	if err != nil {
		return err
	}
	return nil
}

func (p *InternalPlugin) RegisterQueries(engine *ecs.Engine) error {
	err := ecs.RegisterQuery[query.PersonaSignerQueryRequest, query.PersonaSignerQueryResponse](engine, "signer",
		query.PersonaSignerQuery,
		ecs.WithCustomQueryGroup[query.PersonaSignerQueryRequest, query.PersonaSignerQueryResponse]("persona"))
	if err != nil {
		return err
	}
	return nil
}

func (p *InternalPlugin) RegisterSystems(engine *ecs.Engine) error {
	err := engine.RegisterSystems(system.RegisterPersonaSystem, system.AuthorizePersonaAddressSystem)
	if err != nil {
		return err
	}
	return nil
}

func (p *InternalPlugin) RegisterComponents(engine *ecs.Engine) error {
	err := ecs.RegisterComponent[component.SignerComponent](engine)
	if err != nil {
		return err
	}
	return nil
}

func (p *InternalPlugin) RegisterMessages(engine *ecs.Engine) error {
	err := engine.RegisterMessages(msg.CreatePersonaMsg, msg.AuthorizePersonaAddressMsg)
	if err != nil {
		return err
	}
	return nil
}
