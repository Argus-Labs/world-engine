package ecs

import (
	"errors"
	"fmt"

	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/public"
)

// CreatePersonaTransaction allows for the associating of a persona tag with a signer address.
type CreatePersonaTransaction struct {
	PersonaTag    string
	SignerAddress string
}

type CreatePersonaTransactionResult struct {
	Success bool
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
func AuthorizePersonaAddressSystem(world public.IWorld, queue public.ITxQueue, _ public.IWorldLogger) error {
	txs := AuthorizePersonaAddressTx.In(queue)
	if len(txs) == 0 {
		return nil
	}
	personaTagToAddress, err := buildPersonaTagMapping(world)
	if err != nil {
		return err
	}
	for _, tx := range txs {
		if tx.Sig.PersonaTag != tx.Value.PersonaTag {
			AuthorizePersonaAddressTx.AddError(world, tx.TxHash, fmt.Errorf("signer does not match request"))
			AuthorizePersonaAddressTx.SetResult(world, tx.TxHash, AuthorizePersonaAddressResult{Success: false})
			continue
		}
		data, ok := personaTagToAddress[tx.Value.PersonaTag]
		if !ok {
			// This PersonaTag has not been registered.
			AuthorizePersonaAddressTx.AddError(world, tx.TxHash, fmt.Errorf("persona does not exist"))
			AuthorizePersonaAddressTx.SetResult(world, tx.TxHash, AuthorizePersonaAddressResult{Success: false})
			continue
		}
		err = SignerComp.Update(world, data.EntityID, func(component SignerComponent) SignerComponent {
			// check if this address already exists
			for _, addr := range component.AuthorizedAddresses {
				// if its already in the authorized addresses slice, just return the component.
				if addr == tx.Value.Address {
					return component
				}
			}
			component.AuthorizedAddresses = append(component.AuthorizedAddresses, tx.Value.Address)
			return component
		})
		if err != nil {
			AuthorizePersonaAddressTx.AddError(world, tx.TxHash, err)
			AuthorizePersonaAddressTx.SetResult(world, tx.TxHash, AuthorizePersonaAddressResult{Success: false})
			continue
		}
		AuthorizePersonaAddressTx.SetResult(world, tx.TxHash, AuthorizePersonaAddressResult{Success: true})
	}
	return nil
}

type SignerComponent struct {
	PersonaTag          string
	SignerAddress       string
	AuthorizedAddresses []string
}

// SignerComp is the concrete ECS component that pairs a persona tag to a signer address.
var SignerComp = NewComponentType[SignerComponent]("SignerComponent")

type personaTagComponentData struct {
	SignerAddress string
	EntityID      public.EntityID
}

func buildPersonaTagMapping(world public.IWorld) (map[string]personaTagComponentData, error) {
	personaTagToAddress := map[string]personaTagComponentData{}
	var errs []error
	NewQuery(filter.Exact(SignerComp)).Each(world, func(id public.EntityID) bool {
		sc, err := SignerComp.Get(world, id)
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
func RegisterPersonaSystem(world public.IWorld, queue public.ITxQueue, _ public.IWorldLogger) error {
	createTxs := CreatePersonaTx.In(queue)
	if len(createTxs) == 0 {
		return nil
	}
	personaTagToAddress, err := buildPersonaTagMapping(world)
	if err != nil {
		return err
	}
	for _, txData := range createTxs {
		tx := txData.Value
		if _, ok := personaTagToAddress[tx.PersonaTag]; ok {
			// This PersonaTag has already been registered. Don't do anything
			continue
		}
		id, err := world.StoreManager().CreateEntity(SignerComp)
		if err != nil {
			CreatePersonaTx.AddError(world, txData.TxHash, err)
			continue
		}
		if err := SignerComp.Set(world, id, SignerComponent{
			PersonaTag:    tx.PersonaTag,
			SignerAddress: tx.SignerAddress,
		}); err != nil {
			CreatePersonaTx.AddError(world, txData.TxHash, err)
			continue
		}
		personaTagToAddress[tx.PersonaTag] = personaTagComponentData{
			SignerAddress: tx.SignerAddress,
			EntityID:      id,
		}
		CreatePersonaTx.SetResult(world, txData.TxHash, CreatePersonaTransactionResult{
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
	NewQuery(filter.Exact(SignerComp)).Each(w, func(id public.EntityID) bool {
		sc, err := SignerComp.Get(w, id)
		if err != nil {
			errs = append(errs, err)
		}
		if sc.PersonaTag == personaTag {
			addr = sc.SignerAddress
			return false
		}
		return true
	})
	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}

	if addr == "" {
		return "", ErrorPersonaTagHasNoSigner
	}
	return addr, nil
}
