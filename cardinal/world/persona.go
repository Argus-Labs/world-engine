package world

import (
	"errors"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

var (
	ErrPersonaNotRegistered         = eris.New("persona is not registered")
	ErrCreatePersonaTxsNotProcessed = eris.New("create persona txs have not been processed for the given tick")
)

const (
	MinimumPersonaTagLength = 3
	MaximumPersonaTagLength = 16
)

var (
	// Regexp syntax is described here: https://github.com/google/re2/wiki/Syntax
	personaTagRegexp = regexp.MustCompile("^[a-zA-Z0-9_]+$")
)

const (
	PersonaStatusAvailable = "available"
	PersonaStatusAssigned  = "assigned"
)

type PersonaManager struct {
	index *personaIndex
}

func newPersonaManager(w *World) (*PersonaManager, error) {
	pm := &PersonaManager{
		index: nil,
	}

	err := RegisterComponent[Persona](w)
	if err != nil {
		return nil, err
	}

	err = RegisterQuery[PersonaQueryReq, PersonaQueryResp](w, "info", personaQuery,
		WithGroup[PersonaQueryReq, PersonaQueryResp]("persona"))
	if err != nil {
		return nil, err
	}

	err = RegisterMessage[CreatePersona](w)
	if err != nil {
		return nil, err
	}

	err = RegisterMessage[AuthorizePersonaAddress](w)
	if err != nil {
		return nil, err
	}

	err = RegisterSystems(w, createPersonaSystem, authorizePersonaAddressSystem)
	if err != nil {
		return nil, err
	}

	return pm, nil
}

func (pm *PersonaManager) Init(wCtx WorldContextReadOnly) error {
	personaIndex, err := newPersonaIndex(wCtx)
	if err != nil {
		return err
	}
	pm.index = personaIndex
	return nil
}

func (pm *PersonaManager) Get(wCtx WorldContextReadOnly, personaTag string) (*Persona, types.EntityID, error) {
	if pm.index == nil {
		return nil, 0, eris.New("persona index is not initialized")
	}

	entry, err := pm.index.get(personaTag)
	if err == nil {
		persona, err := GetComponent[Persona](wCtx, entry.EntityID)
		if err != nil {
			return nil, 0, err
		}
		return persona, entry.EntityID, nil
	}

	return nil, 0, ErrPersonaNotRegistered
}

// ---------------------------------------------------------------------------------------------------------------------
// Components
// ---------------------------------------------------------------------------------------------------------------------

type Persona struct {
	PersonaTag          string
	SignerAddress       string
	AuthorizedAddresses []string
}

func (Persona) Name() string {
	return "Persona"
}

// ---------------------------------------------------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------------------------------------------------

type AuthorizePersonaAddress struct {
	Address string `json:"address"`
}

func (AuthorizePersonaAddress) Name() string {
	return "persona.authorize-persona-address"
}

// CreatePersona allows for the associating of a persona tag with a signer address.
type CreatePersona struct {
	PersonaTag string `json:"personaTag"`
}

func (CreatePersona) Name() string {
	return "persona.create-persona"
}

// ---------------------------------------------------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------------------------------------------------

// PersonaQueryReq is the desired request body for the query-persona-info endpoint.
type PersonaQueryReq struct {
	PersonaTag string `json:"personaTag"`
}

// PersonaQueryResp is used as the response body for the query-persona-signer endpoint. Status can be:
// "assigned": The requested persona tag has been assigned the returned SignerAddress
// "unknown": The game tick has not advanced far enough to know what the signer address. SignerAddress will be empty.
// "available": The game tick has advanced, and no signer address has been assigned. SignerAddress will be empty.
type PersonaQueryResp struct {
	Status  string  `json:"status"`
	Persona Persona `json:"persona"`
}

func personaQuery(w WorldContextReadOnly, req *PersonaQueryReq) (*PersonaQueryResp, error) {
	persona, _, err := w.GetPersona(req.PersonaTag)
	if err != nil {
		// Handles if the persona tag is not claimed by anyone yet.
		if eris.Is(err, ErrPersonaNotRegistered) {
			return &PersonaQueryResp{
				Status:  PersonaStatusAvailable,
				Persona: Persona{},
			}, nil
		}
		return nil, eris.Wrap(err, "error when fetching persona")
	}

	// Handles if the persona tag has been claimed.
	return &PersonaQueryResp{
		Status:  PersonaStatusAssigned,
		Persona: *persona,
	}, nil
}

// ---------------------------------------------------------------------------------------------------------------------
// Systems
// ---------------------------------------------------------------------------------------------------------------------

type createPersonaResult struct {
	PersonaTag string `json:"personaTag"`
}

// createPersonaSystem is a system that will associate persona tags with signature addresses. Each persona tag
// may have at most 1 signer, so additional attempts to register a signer with a persona tag will be ignored.
func createPersonaSystem(wCtx WorldContext) error {
	return EachMessage[CreatePersona](wCtx, func(tx Tx[CreatePersona]) (any, error) {
		if !IsValidPersonaTag(tx.Msg.PersonaTag) {
			err := eris.Errorf(
				"persona tag %q invalid: must be between %d-%d characters & contain only alphanumeric characters and underscores",
				tx.Msg.PersonaTag,
				MinimumPersonaTagLength,
				MaximumPersonaTagLength)
			return nil, err
		}

		// Normalize the persona tag to lowercase to check against mapping of lowercase tags
		if _, _, err := wCtx.GetPersona(tx.Msg.PersonaTag); err == nil {
			// This PersonaTag has already been registered. Don't do anything
			return nil, eris.Errorf("persona tag %s has already been registered", tx.Msg.PersonaTag)
		}

		id, err := Create(wCtx, Persona{})
		if err != nil {
			return nil, err
		}

		signerHex := ""
		signer, err := tx.Tx.Signer()
		if err != nil {
			if !eris.Is(err, sign.ErrTxNotSigned) {
				wCtx.Logger().Warn().Msg("transaction is not signed, setting persona signer to empty string")
				signerHex = ""
			} else {
				return nil, err
			}
		} else {
			signerHex = signer.Hex()
		}

		if err := SetComponent[Persona](
			wCtx, id, &Persona{
				PersonaTag:          tx.Msg.PersonaTag,
				SignerAddress:       signerHex,
				AuthorizedAddresses: make([]string, 0),
			},
		); err != nil {
			return nil, err
		}

		// Update the index with the new persona
		// TODO: This needs to be reverted when a tick fails to finalize
		err = wCtx.personaManager().index.update(tx.Msg.PersonaTag, signerHex, id)
		if err != nil {
			return nil, err
		}

		return createPersonaResult{PersonaTag: tx.Msg.PersonaTag}, nil
	})
}

type authorizedPersonaAddressResult struct {
	PersonaTag        string `json:"personaTag"`
	AuthorizedAddress string `json:"authorizedAddress"`
}

// authorizePersonaAddressSystem enables users to authorize an address to a persona tag. This is mostly used so that
// users who want to interact with the game via smart contract can link their EVM address to their persona tag, enabling
// them to mutate their owned state from the context of the EVM.
func authorizePersonaAddressSystem(wCtx WorldContext) error {
	return EachMessage[AuthorizePersonaAddress](wCtx, func(tx Tx[AuthorizePersonaAddress]) (any, error) {
		// Check if the Persona Tag exists
		_, id, err := wCtx.GetPersona(tx.Tx.PersonaTag)
		if err != nil {
			return nil, eris.Errorf("persona %s does not exist", tx.Tx.PersonaTag)
		}

		// Normalize address
		tx.Msg.Address = strings.ToLower(tx.Msg.Address)
		tx.Msg.Address = strings.ReplaceAll(tx.Msg.Address, " ", "")

		// Check that address is valid
		valid := common.IsHexAddress(tx.Msg.Address)
		if !valid {
			return nil, eris.Errorf("address %s is invalid", tx.Msg.Address)
		}

		err = UpdateComponent[Persona](wCtx, id, func(s *Persona) *Persona {
			for _, addr := range s.AuthorizedAddresses {
				if addr == tx.Msg.Address {
					return s
				}
			}
			s.AuthorizedAddresses = append(s.AuthorizedAddresses, tx.Msg.Address)
			return s
		})
		if err != nil {
			return nil, eris.Wrap(err, "unable to update persona component")
		}

		return authorizedPersonaAddressResult{
			PersonaTag:        tx.Tx.PersonaTag,
			AuthorizedAddress: tx.Msg.Address,
		}, nil
	})
}

// ---------------------------------------------------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------------------------------------------------

// IsValidPersonaTag checks that string is a valid persona tag: alphanumeric + underscore
func IsValidPersonaTag(s string) bool {
	if length := len(s); length < MinimumPersonaTagLength || length > MaximumPersonaTagLength {
		return false
	}
	return personaTagRegexp.MatchString(s)
}

// ---------------------------------------------------------------------------------------------------------------------
// Index
// ---------------------------------------------------------------------------------------------------------------------

type personaIndexEntry struct {
	SignerAddress string
	EntityID      types.EntityID
}

type personaIndex struct {
	index map[string]personaIndexEntry
}

func newPersonaIndex(wCtx WorldContextReadOnly) (*personaIndex, error) {
	index := map[string]personaIndexEntry{}

	var errs []error
	err := wCtx.Search(filter.Exact(filter.Component[Persona]())).Each(func(id types.EntityID) bool {
		sc, err := GetComponent[Persona](wCtx, id)
		if err != nil {
			errs = append(errs, err)
			// Terminate the iteration
			return false
		}

		// Normalize the persona tag to lowercase
		personaTag := strings.ToLower(sc.PersonaTag)
		index[personaTag] = personaIndexEntry{
			SignerAddress: sc.SignerAddress,
			EntityID:      id,
		}

		// Continue the iteration
		return true
	})
	if err != nil {
		return nil, err
	}
	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}

	return &personaIndex{index: index}, nil
}

func (p *personaIndex) update(personaTag string, signer string, entityID types.EntityID) error {
	personaTag = strings.ToLower(personaTag)
	p.index[personaTag] = personaIndexEntry{
		SignerAddress: signer,
		EntityID:      entityID,
	}
	return nil
}

func (p *personaIndex) get(personaTag string) (*personaIndexEntry, error) {
	personaTag = strings.ToLower(personaTag)
	entry, ok := p.index[personaTag]
	if !ok {
		return nil, eris.Errorf("persona tag %q not found in index", personaTag)
	}
	return &entry, nil
}
