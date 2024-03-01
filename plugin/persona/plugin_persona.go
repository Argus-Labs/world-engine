package persona

import (
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/plugin/persona/component"
	"pkg.world.dev/world-engine/plugin/persona/msg"
	"pkg.world.dev/world-engine/plugin/persona/query"
)

type personaPlugin struct {
}

func newPersonaPlugin() *personaPlugin {
	return &personaPlugin{}
}

var _ cardinal.Plugin = (*personaPlugin)(nil)

func (p *personaPlugin) Register(world *cardinal.World) error {
	err := p.registerQueries(world)
	if err != nil {
		return err
	}
	err = p.registerSystems(world)
	if err != nil {
		return err
	}
	err = p.registerComponents(world)
	if err != nil {
		return err
	}
	err = p.registerMessages(world)
	if err != nil {
		return err
	}
	return nil
}

func (p *personaPlugin) registerQueries(world *cardinal.World) error {
	err := cardinal.RegisterQuery[query.PersonaSignerQueryRequest, query.PersonaSignerQueryResponse](world, "signer",
		query.PersonaSignerQuery,
		cardinal.WithCustomQueryGroup[query.PersonaSignerQueryRequest, query.PersonaSignerQueryResponse]("persona"))
	if err != nil {
		return err
	}
	return nil
}

func (p *personaPlugin) registerSystems(world *cardinal.World) error {
	err := cardinal.RegisterSystems(world, RegisterPersonaSystem, AuthorizePersonaAddressSystem)
	if err != nil {
		return err
	}
	return nil
}

func (p *personaPlugin) registerComponents(world *cardinal.World) error {
	err := cardinal.RegisterComponent[component.SignerComponent](world)
	if err != nil {
		return err
	}
	return nil
}

func (p *personaPlugin) registerMessages(world *cardinal.World) error {
	return errors.Join(
		cardinal.RegisterMessage[msg.CreatePersona, msg.CreatePersonaResult](
			world,
			"create-persona",
			message.WithCustomMessageGroup[msg.CreatePersona, msg.CreatePersonaResult]("persona"),
			message.WithMsgEVMSupport[msg.CreatePersona, msg.CreatePersonaResult]()),
		cardinal.RegisterMessage[msg.AuthorizePersonaAddress, msg.AuthorizePersonaAddressResult](
			world,
			"authorize-persona-address",
		))
}

// -----------------------------------------------------------------------------
// Persona Messages
// -----------------------------------------------------------------------------

// AuthorizePersonaAddressSystem enables users to authorize an address to a persona tag. This is mostly used so that
// users who want to interact with the game via smart contract can link their EVM address to their persona tag, enabling
// them to mutate their owned state from the context of the EVM.
func AuthorizePersonaAddressSystem(wCtx engine.Context) error {
	personaTagToAddress, err := buildPersonaIndex(wCtx)
	if err != nil {
		return err
	}
	return cardinal.EachMessage[msg.AuthorizePersonaAddress, msg.AuthorizePersonaAddressResult](
		wCtx,
		func(txData message.TxData[msg.AuthorizePersonaAddress]) (
			result msg.AuthorizePersonaAddressResult, err error,
		) {
			txMsg, tx := txData.Msg, txData.Tx
			result.Success = false

			// Check if the Persona Tag exists
			lowerPersona := strings.ToLower(tx.PersonaTag)
			data, ok := personaTagToAddress[lowerPersona]
			if !ok {
				return result, eris.Errorf("persona %s does not exist", tx.PersonaTag)
			}

			// Check that the ETH Address is valid
			txMsg.Address = strings.ToLower(txMsg.Address)
			txMsg.Address = strings.ReplaceAll(txMsg.Address, " ", "")
			valid := common.IsHexAddress(txMsg.Address)
			if !valid {
				return result, eris.Errorf("eth address %s is invalid", txMsg.Address)
			}

			err = cardinal.UpdateComponent[component.SignerComponent](
				wCtx, data.EntityID, func(s *component.SignerComponent) *component.SignerComponent {
					for _, addr := range s.AuthorizedAddresses {
						if addr == txMsg.Address {
							return s
						}
					}
					s.AuthorizedAddresses = append(s.AuthorizedAddresses, txMsg.Address)
					return s
				},
			)
			if err != nil {
				return result, eris.Wrap(err, "unable to update signer component with address")
			}
			result.Success = true
			return result, nil
		},
	)
}

// -----------------------------------------------------------------------------
// Persona System
// -----------------------------------------------------------------------------

// RegisterPersonaSystem is an system that will associate persona tags with signature addresses. Each persona tag
// may have at most 1 signer, so additional attempts to register a signer with a persona tag will be ignored.
func RegisterPersonaSystem(wCtx engine.Context) error {
	personaTagToAddress, err := buildPersonaIndex(wCtx)
	if err != nil {
		return err
	}
	return cardinal.EachMessage[msg.CreatePersona, msg.CreatePersonaResult](
		wCtx,
		func(txData message.TxData[msg.CreatePersona]) (result msg.CreatePersonaResult, err error) {
			txMsg := txData.Msg
			result.Success = false

			if !IsValidPersonaTag(txMsg.PersonaTag) {
				err = eris.Errorf("persona tag %s is not valid: must only contain alphanumerics and underscores",
					txMsg.PersonaTag)
				return result, err
			}

			// Temporarily convert tag to lowercase to check against mapping of lowercase tags
			lowerPersona := strings.ToLower(txMsg.PersonaTag)
			if _, ok := personaTagToAddress[lowerPersona]; ok {
				// This PersonaTag has already been registered. Don't do anything
				err = eris.Errorf("persona tag %s has already been registered", txMsg.PersonaTag)
				return result, err
			}
			id, err := cardinal.Create(wCtx, component.SignerComponent{})
			if err != nil {
				return result, eris.Wrap(err, "")
			}
			if err = cardinal.SetComponent[component.SignerComponent](
				wCtx, id, &component.SignerComponent{
					PersonaTag:    txMsg.PersonaTag,
					SignerAddress: txMsg.SignerAddress,
				},
			); err != nil {
				return result, eris.Wrap(err, "")
			}
			personaTagToAddress[lowerPersona] = personaIndexEntry{
				SignerAddress: txMsg.SignerAddress,
				EntityID:      id,
			}
			result.Success = true
			return result, nil
		},
	)
}

// -----------------------------------------------------------------------------
// Persona Index
// -----------------------------------------------------------------------------

type personaIndex = map[string]personaIndexEntry

type personaIndexEntry struct {
	SignerAddress string
	EntityID      types.EntityID
}

func buildPersonaIndex(wCtx engine.Context) (personaIndex, error) {
	personaTagToAddress := map[string]personaIndexEntry{}
	var errs []error
	err := cardinal.NewSearch(wCtx, filter.Exact(component.SignerComponent{})).Each(
		func(id types.EntityID) bool {
			sc, err := cardinal.GetComponent[component.SignerComponent](wCtx, id)
			if err != nil {
				errs = append(errs, err)
				return true
			}
			lowerPersona := strings.ToLower(sc.PersonaTag)
			personaTagToAddress[lowerPersona] = personaIndexEntry{
				SignerAddress: sc.SignerAddress,
				EntityID:      id,
			}
			return true
		},
	)
	if err != nil {
		return nil, err
	}
	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}
	return personaTagToAddress, nil
}
