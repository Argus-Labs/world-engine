package ecs

import (
	"errors"
	"pkg.world.dev/world-engine/cardinal/ecs/search"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"regexp"
	"strings"

	"pkg.world.dev/world-engine/cardinal/ecs/filter"

	"github.com/ethereum/go-ethereum/common"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

const (
	getSignerForPersonaStatusUnknown   = "unknown"
	getSignerForPersonaStatusAvailable = "available"
	getSignerForPersonaStatusAssigned  = "assigned"
)

// QueryPersonaSignerRequest is the desired request body for the query-persona-signer endpoint.
type QueryPersonaSignerRequest struct {
	PersonaTag string `json:"personaTag"`
	Tick       uint64 `json:"tick"`
}

// QueryPersonaSignerResponse is used as the response body for the query-persona-signer endpoint. Status can be:
// "assigned": The requested persona tag has been assigned the returned SignerAddress
// "unknown": The game tick has not advanced far enough to know what the signer address. SignerAddress will be empty.
// "available": The game tick has advanced, and no signer address has been assigned. SignerAddress will be empty.
type QueryPersonaSignerResponse struct {
	Status        string `json:"status"`
	SignerAddress string `json:"signerAddress"`
}

// querySigner godoc
//
//	@Summary		Get persona data from cardinal
//	@Description	Get persona data from cardinal
//	@Accept			application/json
//	@Produce		application/json
//	@Param			QueryPersonaSignerRequest	body		QueryPersonaSignerRequest	true	"Query Request"
//	@Success		200							{object}	QueryPersonaSignerResponse
//	@Failure		400							{string}	string	"Invalid query request"
//	@Router			/query/persona/signer [post]
func querySigner(eCtx EngineContext, req *QueryPersonaSignerRequest) (*QueryPersonaSignerResponse, error) {
	var status string

	addr, err := eCtx.GetSignerForPersonaTag(req.PersonaTag, req.Tick)
	if err != nil {
		//nolint:gocritic // cant switch case this.
		if errors.Is(err, ErrPersonaTagHasNoSigner) {
			status = getSignerForPersonaStatusAvailable
		} else if errors.Is(err, ErrCreatePersonaTxsNotProcessed) {
			status = getSignerForPersonaStatusUnknown
		} else {
			return nil, err
		}
	} else {
		status = getSignerForPersonaStatusAssigned
	}

	res := QueryPersonaSignerResponse{
		Status:        status,
		SignerAddress: addr,
	}
	return &res, nil
}

// CreatePersona allows for the associating of a persona tag with a signer address.
type CreatePersona struct {
	PersonaTag    string `json:"personaTag"`
	SignerAddress string `json:"signerAddress"`
}

type CreatePersonaResult struct {
	Success bool `json:"success"`
}

// CreatePersonaMsg is a message that facilitates the creation of a persona tag.
var CreatePersonaMsg = NewMessageType[CreatePersona, CreatePersonaResult](
	"create-persona",
	WithMsgEVMSupport[CreatePersona, CreatePersonaResult](),
	WithCustomMessageGroup[CreatePersona, CreatePersonaResult]("persona"),
)

var regexpObj = regexp.MustCompile("^[a-zA-Z0-9_]+$")

type AuthorizePersonaAddress struct {
	Address string `json:"address"`
}

type AuthorizePersonaAddressResult struct {
	Success bool `json:"success"`
}

var AuthorizePersonaAddressMsg = NewMessageType[AuthorizePersonaAddress, AuthorizePersonaAddressResult](
	"authorize-persona-address",
)

// AuthorizePersonaAddressSystem enables users to authorize an address to a persona tag. This is mostly used so that
// users who want to interact with the game via smart contract can link their EVM address to their persona tag, enabling
// them to mutate their owned state from the context of the EVM.
func AuthorizePersonaAddressSystem(eCtx engine.Context) error {
	personaTagToAddress, err := buildPersonaTagMapping(eCtx)
	if err != nil {
		return err
	}

	AuthorizePersonaAddressMsg.Each(
		eCtx, func(txData TxData[AuthorizePersonaAddress]) (result AuthorizePersonaAddressResult, err error) {
			msg, tx := txData.Msg, txData.Tx
			result.Success = false

			// Check if the Persona Tag exists
			lowerPersona := strings.ToLower(tx.PersonaTag)
			data, ok := personaTagToAddress[lowerPersona]
			if !ok {
				return result, eris.Errorf("persona %s does not exist", tx.PersonaTag)
			}

			// Check that the ETH Address is valid
			msg.Address = strings.ToLower(msg.Address)
			msg.Address = strings.ReplaceAll(msg.Address, " ", "")
			valid := common.IsHexAddress(msg.Address)
			if !valid {
				return result, eris.Errorf("eth address %s is invalid", msg.Address)
			}

			err = UpdateComponent[SignerComponent](
				eCtx, data.EntityID, func(s *SignerComponent) *SignerComponent {
					for _, addr := range s.AuthorizedAddresses {
						if addr == msg.Address {
							return s
						}
					}
					s.AuthorizedAddresses = append(s.AuthorizedAddresses, msg.Address)
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
	return nil
}

type SignerComponent struct {
	PersonaTag          string
	SignerAddress       string
	AuthorizedAddresses []string
}

func (SignerComponent) Name() string {
	return "SignerComponent"
}

type personaTagComponentData struct {
	SignerAddress string
	EntityID      entity.ID
}

func buildPersonaTagMapping(eCtx engine.Context) (map[string]personaTagComponentData, error) {
	personaTagToAddress := map[string]personaTagComponentData{}
	var errs []error
	q := search.NewSearch(filter.Exact(SignerComponent{}), eCtx.Namespace(), eCtx.StoreReader())
	err := q.Each(
		func(id entity.ID) bool {
			sc, err := GetComponent[SignerComponent](eCtx, id)
			if err != nil {
				errs = append(errs, err)
				return true
			}
			lowerPersona := strings.ToLower(sc.PersonaTag)
			personaTagToAddress[lowerPersona] = personaTagComponentData{
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

// RegisterPersonaSystem is an ecs.System that will associate persona tags with signature addresses. Each persona tag
// may have at most 1 signer, so additional attempts to register a signer with a persona tag will be ignored.
func RegisterPersonaSystem(eCtx engine.Context) error {
	personaTagToAddress, err := buildPersonaTagMapping(eCtx)
	if err != nil {
		return err
	}

	CreatePersonaMsg.Each(eCtx, func(txData TxData[CreatePersona]) (result CreatePersonaResult, err error) {
		msg := txData.Msg
		result.Success = false

		if !isAlphanumericWithUnderscore(msg.PersonaTag) {
			err = eris.Errorf("persona tag %s is not valid: must only contain alphanumerics and underscores",
				msg.PersonaTag)
			return result, err
		}

		// Temporarily convert tag to lowercase to check against mapping of lowercase tags
		lowerPersona := strings.ToLower(msg.PersonaTag)
		if _, ok := personaTagToAddress[lowerPersona]; ok {
			// This PersonaTag has already been registered. Don't do anything
			err = eris.Errorf("persona tag %s has already been registered", msg.PersonaTag)
			return result, err
		}
		id, err := Create(eCtx, SignerComponent{})
		if err != nil {
			return result, eris.Wrap(err, "")
		}
		if err = SetComponent[SignerComponent](
			eCtx, id, &SignerComponent{
				PersonaTag:    msg.PersonaTag,
				SignerAddress: msg.SignerAddress,
			},
		); err != nil {
			return result, eris.Wrap(err, "")
		}
		personaTagToAddress[lowerPersona] = personaTagComponentData{
			SignerAddress: msg.SignerAddress,
			EntityID:      id,
		}
		result.Success = true
		return result, nil
	})

	return nil
}

func isAlphanumericWithUnderscore(s string) bool {
	// Use the MatchString method to check if the string matches the pattern
	return regexpObj.MatchString(s)
}

var (
	ErrPersonaTagHasNoSigner        = errors.New("persona tag does not have a signer")
	ErrCreatePersonaTxsNotProcessed = errors.New("create persona txs have not been processed for the given tick")
)

// GetSignerForPersonaTag returns the signer address that has been registered for the given persona tag after the
// given tick. If the engine's tick is less than or equal to the given tick, ErrorCreatePersonaTXsNotProcessed is
// returned. If the given personaTag has no signer address, ErrPersonaTagHasNoSigner is returned.
func (e *Engine) GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error) {
	if tick >= e.CurrentTick() {
		return "", ErrCreatePersonaTxsNotProcessed
	}
	var errs []error
	q := e.NewSearch(filter.Exact(SignerComponent{}))
	eCtx := NewReadOnlyEngineContext(e)
	err = q.Each(
		func(id entity.ID) bool {
			sc, err := GetComponent[SignerComponent](eCtx, id)
			if err != nil {
				errs = append(errs, err)
			}
			if sc.PersonaTag == personaTag {
				addr = sc.SignerAddress
				return false
			}
			return true
		},
	)
	errs = append(errs, err)
	if addr == "" {
		return "", ErrPersonaTagHasNoSigner
	}
	return addr, errors.Join(errs...)
}
