package ecs

import (
	"errors"
	"fmt"
	"strconv"

	"pkg.world.dev/world-engine/cardinal/ecs/component_metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

// CreatePersonaTransaction allows for the associating of a persona tag with a signer address.
type CreatePersonaTransaction struct {
	PersonaTag    string `json:"personaTag"`
	SignerAddress string `json:"signerAddress"`
}

type CreatePersonaTransactionResult struct {
	Success bool `json:"success"`
}

// CreatePersonaTx is a concrete ECS transaction.
var CreatePersonaTx = NewTransactionType[CreatePersonaTransaction, CreatePersonaTransactionResult](
	"create-persona",
	WithTxEVMSupport[CreatePersonaTransaction, CreatePersonaTransactionResult],
)

type AuthorizePersonaAddress struct {
	PersonaTag string
	Address    string
}

type AuthorizePersonaAddressResult struct {
	Success bool
}

var AuthorizePersonaAddressTx = NewTransactionType[AuthorizePersonaAddress, AuthorizePersonaAddressResult](
	"authorize-persona-address",
)

// AuthorizePersonaAddressSystem enables users to authorize an address to a persona tag. This is mostly used so that
// users who want to interact with the game via smart contract can link their EVM address to their persona tag, enabling
// them to mutate their owned state from the context of the EVM.
func AuthorizePersonaAddressSystem(wCtx WorldContext) error {
	personaTagToAddress, err := buildPersonaTagMapping(wCtx)
	if err != nil {
		return err
	}
	AuthorizePersonaAddressTx.ForEach(wCtx, func(tx TxData[AuthorizePersonaAddress]) (AuthorizePersonaAddressResult, error) {
		val, sig := tx.Value, tx.Sig
		result := AuthorizePersonaAddressResult{Success: false}
		if sig.PersonaTag != val.PersonaTag {
			return AuthorizePersonaAddressResult{Success: false}, fmt.Errorf("sigher does not match request")
		}
		data, ok := personaTagToAddress[tx.Value.PersonaTag]
		if !ok {
			return result, fmt.Errorf("persona does not exist")
		}
		err = updateComponent[SignerComponent](wCtx, data.EntityID, func(s *SignerComponent) *SignerComponent {
			for _, addr := range s.AuthorizedAddresses {
				if addr == val.Address {
					return s
				}
			}
			s.AuthorizedAddresses = append(s.AuthorizedAddresses, val.Address)
			return s
		})
		if err != nil {
			return result, fmt.Errorf("unable to update signer component with address: %w", err)
		}
		result.Success = true
		return result, nil
	})
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

func buildPersonaTagMapping(wCtx WorldContext) (map[string]personaTagComponentData, error) {
	personaTagToAddress := map[string]personaTagComponentData{}
	var errs []error
	q, err := wCtx.NewSearch(Exact(SignerComponent{}))
	if err != nil {
		return nil, err
	}
	q.Each(wCtx, func(id entity.ID) bool {
		sc, err := getComponent[SignerComponent](wCtx, id)
		if err != nil {
			errs = append(errs, err)
			return true
		}
		personaTagToAddress[sc.PersonaTag] = personaTagComponentData{
			SignerAddress: sc.SignerAddress,
			EntityID:      id,
		}
		return true
	})
	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}
	return personaTagToAddress, nil
}

// RegisterPersonaSystem is an ecs.System that will associate persona tags with signature addresses. Each persona tag
// may have at most 1 signer, so additional attempts to register a signer with a persona tag will be ignored.
func RegisterPersonaSystem(wCtx WorldContext) error {
	createTxs := CreatePersonaTx.In(wCtx)
	if len(createTxs) == 0 {
		return nil
	}
	personaTagToAddress, err := buildPersonaTagMapping(wCtx)
	if err != nil {
		return err
	}
	for _, txData := range createTxs {
		tx := txData.Value
		if _, ok := personaTagToAddress[tx.PersonaTag]; ok {
			// This PersonaTag has already been registered. Don't do anything
			continue
		}
		id, err := create(wCtx, SignerComponent{})
		if err != nil {
			CreatePersonaTx.AddError(wCtx, txData.TxHash, err)
			continue
		}
		if err := setComponent[SignerComponent](wCtx, id, &SignerComponent{
			PersonaTag:    tx.PersonaTag,
			SignerAddress: tx.SignerAddress,
		}); err != nil {
			CreatePersonaTx.AddError(wCtx, txData.TxHash, err)
			continue
		}
		personaTagToAddress[tx.PersonaTag] = personaTagComponentData{
			SignerAddress: tx.SignerAddress,
			EntityID:      id,
		}
		CreatePersonaTx.SetResult(wCtx, txData.TxHash, CreatePersonaTransactionResult{
			Success: true,
		})
	}

	return nil
}

var (
	ErrorPersonaTagHasNoSigner        = errors.New("persona tag does not have a signer")
	ErrorCreatePersonaTxsNotProcessed = errors.New("create persona txs have not been processed for the given tick")
)

// GetSignerForPersonaTag returns the signer address that has been registered for the given persona tag after the
// given tick. If the world's tick is less than or equal to the given tick, ErrorCreatePersonaTXsNotProcessed is returned.
// If the given personaTag has no signer address, ErrorPersonaTagHasNoSigner is returned.
func (w *World) GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error) {
	if tick >= w.tick {
		return "", ErrorCreatePersonaTxsNotProcessed
	}
	var errs []error
	q, err := w.NewSearch(Exact(SignerComponent{}))
	if err != nil {
		return "", err
	}
	wCtx := NewReadOnlyWorldContext(w)
	err = q.Each(wCtx, func(id entity.ID) bool {
		sc, err := getComponent[SignerComponent](wCtx, id)
		//sc, err := SignerComp.Get(w, id)
		if err != nil {
			errs = append(errs, err)
		}
		if sc.PersonaTag == personaTag {
			addr = sc.SignerAddress
			return false
		}
		return true
	})
	errs = append(errs, err)
	if addr == "" {
		return "", ErrorPersonaTagHasNoSigner
	}
	return addr, errors.Join(errs...)
}

// TODO private component function used to temporarily remove circular dependency until we replace components.
// TODO this function is intended only for use with persona.go and is to be removed with persona when we replace with plugins.
// Get returns component data from the entity.
// GetComponent returns component data from the entity.
func getComponent[T component_metadata.Component](wCtx WorldContext, id entity.ID) (comp *T, err error) {
	var t T
	name := t.Name()
	c, err := wCtx.GetWorld().GetComponentByName(name)
	if err != nil {
		return nil, errors.New("Must register component")
	}
	value, err := wCtx.StoreReader().GetComponentForEntity(c, id)
	if err != nil {
		return nil, err
	}
	t, ok := value.(T)
	if !ok {
		comp, ok = value.(*T)
		if !ok {
			return nil, fmt.Errorf("type assertion for component failed: %v to %v", value, c)
		}
	} else {
		comp = &t
	}

	return comp, nil
}

// TODO private component function used to temporarily remove circular dependency until we replace components.
// TODO this function is intended only for use with persona.go and is to be removed with persona when we replace with plugins.
// Set sets component data to the entity.
func setComponent[T component_metadata.Component](wCtx WorldContext, id entity.ID, component *T) error {
	if wCtx.IsReadOnly() {
		return ErrorCannotModifyStateWithReadOnlyContext
	}
	var t T
	name := t.Name()
	c, err := wCtx.GetWorld().GetComponentByName(name)
	if err != nil {
		return fmt.Errorf("%s is not registered, please register it before updating", t.Name())
	}
	err = wCtx.StoreManager().SetComponentForEntity(c, id, component)
	if err != nil {
		return err
	}
	wCtx.Logger().Debug().
		Str("entity_id", strconv.FormatUint(uint64(id), 10)).
		Str("component_name", c.Name()).
		Int("component_id", int(c.ID())).
		Msg("entity updated")
	return nil
}

// TODO private component function used to temporarily remove circular dependency until we replace components.
// TODO this function is intended only for use with persona.go and is to be removed with persona when we replace with plugins.
// https://linear.app/arguslabs/issue/WORLD-423/ecs-plugin-feature
func updateComponent[T component_metadata.Component](wCtx WorldContext, id entity.ID, fn func(*T) *T) error {
	if wCtx.IsReadOnly() {
		return ErrorCannotModifyStateWithReadOnlyContext
	}
	val, err := getComponent[T](wCtx, id)
	if err != nil {
		return err
	}
	updatedVal := fn(val)
	return setComponent[T](wCtx, id, updatedVal)
}

// TODO private component function used to temporarily remove circular dependency until we replace components.
// TODO this function is intended only for use with persona.go and is to be removed with persona when we replace with plugins.
// https://linear.app/arguslabs/issue/WORLD-423/ecs-plugin-feature
func createMany(wCtx WorldContext, num int, components ...component_metadata.Component) ([]entity.ID, error) {
	if wCtx.IsReadOnly() {
		return nil, ErrorCannotModifyStateWithReadOnlyContext
	}
	world := wCtx.GetWorld()
	acc := make([]component_metadata.IComponentMetaData, 0, len(components))
	for _, comp := range components {
		c, err := world.GetComponentByName(comp.Name())
		if err != nil {
			return nil, err
		}
		acc = append(acc, c)
	}
	entityIds, err := world.StoreManager().CreateManyEntities(num, acc...)
	if err != nil {
		return nil, err
	}
	for _, id := range entityIds {
		for _, comp := range components {
			c, err := world.GetComponentByName(comp.Name())
			if err != nil {
				return nil, errors.New("Must register component before creating an entity")
			}
			err = world.StoreManager().SetComponentForEntity(c, id, comp)
			if err != nil {
				return nil, err
			}
		}
	}
	return entityIds, nil
}

// TODO private component function used to temporarily remove circular dependency until we replace components.
// TODO this function is intended only for use with persona.go and is to be removed with persona when we replace with plugins.
// https://linear.app/arguslabs/issue/WORLD-423/ecs-plugin-feature
func create(wCtx WorldContext, components ...component_metadata.Component) (entity.ID, error) {
	entities, err := createMany(wCtx, 1, components...)
	if err != nil {
		return 0, err
	}
	return entities[0], nil
}
