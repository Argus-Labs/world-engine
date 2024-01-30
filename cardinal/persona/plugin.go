package persona

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/persona/query"
	"pkg.world.dev/world-engine/cardinal/persona/system"
)

type InternalPlugin struct{}

var _ ecs.InternalPlugin = (*InternalPlugin)(nil)

func NewInternalPlugin() *InternalPlugin {
	return &InternalPlugin{}
}

func (p *InternalPlugin) Register(engine *ecs.Engine) error {
	err := RegisterQueries(engine)
	if err != nil {
		return err
	}
	err = RegisterSystems(engine)
	if err != nil {
		return err
	}
	err = RegisterComponents(engine)
	if err != nil {
		return err
	}
	err = RegisterMessages(engine)
	if err != nil {
		return err
	}
	return nil
}

func RegisterQueries(engine *ecs.Engine) error {
	err := ecs.RegisterQuery[query.QueryPersonaSignerRequest, query.QueryPersonaSignerResponse](engine, "signer",
		query.QueryPersonaSigner,
		ecs.WithCustomQueryGroup[query.QueryPersonaSignerRequest, query.QueryPersonaSignerResponse]("persona"))
	if err != nil {
		return err
	}
	return nil
}

func RegisterSystems(engine *ecs.Engine) error {
	err := engine.RegisterSystems(system.RegisterPersonaSystem, system.AuthorizePersonaAddressSystem)
	if err != nil {
		return err
	}
	return nil
}

func RegisterComponents(engine *ecs.Engine) error {
	err := ecs.RegisterComponent[component.SignerComponent](engine)
	if err != nil {
		return err
	}
	return nil
}

func RegisterMessages(engine *ecs.Engine) error {
	err := engine.RegisterMessages(msg.CreatePersonaMsg, msg.AuthorizePersonaAddressMsg)
	if err != nil {
		return err
	}
	return nil
}
