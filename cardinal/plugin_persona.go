package cardinal

import (
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/persona"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/persona/query"
	"pkg.world.dev/world-engine/cardinal/search"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

var (
	_ Plugin = (*personaPlugin)(nil)

	// TODO: Replace these global variables when indexing/fast-searching is supported.
	// See https://linear.app/arguslabs/issue/WORLD-1057/spec-out-component-indexing
	// These global variables are used to quickly identify already-created persona tags. The map should exactly match
	// the persona tag information stored in the ECS layer. When Cardinal restarts, this map needs to be rebuilt.
	//
	// globalPersonaTagToAddressIndex keeps track of the mapping of persona-tags->signer-address so it doesn't need to
	// be recomputed each tick.
	globalPersonaTagToAddressIndex personaIndex
	// tickOfPersonaTagToAddressIndex is the tick that the globalPersonaTagToAddressIndex was built on. In normal usage,
	// wCtx.CurrentTick should always be greater than this number, but during tests the currentTick will be reset.
	// Tracking this number at the global is easier than updating each test to reset these global value.
	tickOfPersonaTagToAddressIndex uint64
)

type personaIndex = map[string]personaIndexEntry

type personaIndexEntry struct {
	SignerAddress string
	EntityID      types.EntityID
}

type personaPlugin struct {
}

func newPersonaPlugin() *personaPlugin {
	return &personaPlugin{}
}

func (p *personaPlugin) Register(world *World) error {
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

func (p *personaPlugin) RegisterQueries(world *World) error {
	err := RegisterQuery[query.PersonaSignerQueryRequest, query.PersonaSignerQueryResponse](world, "signer",
		query.PersonaSignerQuery,
		WithCustomQueryGroup[query.PersonaSignerQueryRequest, query.PersonaSignerQueryResponse]("persona"))
	if err != nil {
		return err
	}
	return nil
}

func (p *personaPlugin) RegisterSystems(world *World) error {
	err := RegisterSystems(world, CreatePersonaSystem, AuthorizePersonaAddressSystem)
	if err != nil {
		return err
	}
	return nil
}

func (p *personaPlugin) RegisterComponents(world *World) error {
	err := RegisterComponent[component.SignerComponent](world)
	if err != nil {
		return err
	}
	return nil
}

func (p *personaPlugin) RegisterMessages(world *World) error {
	return errors.Join(
		RegisterMessage[msg.CreatePersona, msg.CreatePersonaResult](
			world,
			msg.CreatePersonaMessageName,
			WithCustomMessageGroup[msg.CreatePersona, msg.CreatePersonaResult]("persona"),
			WithMsgEVMSupport[msg.CreatePersona, msg.CreatePersonaResult]()),
		RegisterMessage[msg.AuthorizePersonaAddress, msg.AuthorizePersonaAddressResult](
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
	if err := buildGlobalPersonaIndex(wCtx); err != nil {
		return err
	}
	return EachMessage[msg.AuthorizePersonaAddress, msg.AuthorizePersonaAddressResult](
		wCtx,
		func(txData TxData[msg.AuthorizePersonaAddress]) (
			result msg.AuthorizePersonaAddressResult, err error,
		) {
			txMsg, tx := txData.Msg, txData.Tx
			result.Success = false

			// Check if the Persona Tag exists
			lowerPersona := strings.ToLower(tx.PersonaTag)
			data, ok := globalPersonaTagToAddressIndex[lowerPersona]
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

			err = UpdateComponent[component.SignerComponent](
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

// CreatePersonaSystem is a system that will associate persona tags with signature addresses. Each persona tag
// may have at most 1 signer, so additional attempts to register a signer with a persona tag will be ignored.
func CreatePersonaSystem(wCtx engine.Context) error {
	if err := buildGlobalPersonaIndex(wCtx); err != nil {
		return err
	}
	return EachMessage[msg.CreatePersona, msg.CreatePersonaResult](
		wCtx,
		func(txData TxData[msg.CreatePersona]) (result msg.CreatePersonaResult, err error) {
			txMsg := txData.Msg
			result.Success = false

			if !persona.IsValidPersonaTag(txMsg.PersonaTag) {
				err := eris.Errorf(
					"persona tag %q invalid: must be between %d-%d characters & contain only alphanumeric characters and underscores",
					txMsg.PersonaTag,
					persona.MinimumPersonaTagLength,
					persona.MaximumPersonaTagLength)
				return result, err
			}

			// Temporarily convert tag to lowercase to check against mapping of lowercase tags
			lowerPersona := strings.ToLower(txMsg.PersonaTag)
			if _, ok := globalPersonaTagToAddressIndex[lowerPersona]; ok {
				// This PersonaTag has already been registered. Don't do anything
				err = eris.Errorf("persona tag %s has already been registered", txMsg.PersonaTag)
				return result, err
			}
			id, err := Create(wCtx, component.SignerComponent{})
			if err != nil {
				return result, eris.Wrap(err, "")
			}
			if err = SetComponent[component.SignerComponent](
				wCtx, id, &component.SignerComponent{
					PersonaTag:          txMsg.PersonaTag,
					SignerAddress:       txMsg.SignerAddress,
					AuthorizedAddresses: make([]string, 0),
				},
			); err != nil {
				return result, eris.Wrap(err, "")
			}
			globalPersonaTagToAddressIndex[lowerPersona] = personaIndexEntry{
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

func buildGlobalPersonaIndex(wCtx engine.Context) error {
	// Rebuild the index if we haven't built it yet OR if we're in test and the CurrentTick has been reset.
	if globalPersonaTagToAddressIndex != nil && tickOfPersonaTagToAddressIndex < wCtx.CurrentTick() {
		return nil
	}
	tickOfPersonaTagToAddressIndex = wCtx.CurrentTick()
	globalPersonaTagToAddressIndex = map[string]personaIndexEntry{}
	var errs []error
	s := search.NewSearch().Entity(filter.Exact(filter.Component[component.SignerComponent]()))
	err := s.Each(wCtx,
		func(id types.EntityID) bool {
			sc, err := GetComponent[component.SignerComponent](wCtx, id)
			if err != nil {
				errs = append(errs, err)
				return true
			}
			lowerPersona := strings.ToLower(sc.PersonaTag)
			globalPersonaTagToAddressIndex[lowerPersona] = personaIndexEntry{
				SignerAddress: sc.SignerAddress,
				EntityID:      id,
			}
			return true
		},
	)
	if err != nil {
		return err
	}
	if len(errs) != 0 {
		return errors.Join(errs...)
	}
	return nil
}
