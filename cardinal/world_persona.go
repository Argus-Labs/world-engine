package cardinal

import (
	"errors"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/types"
)

var (
	ErrPersonaTagHasNoSigner        = errors.New("persona tag does not have a signer")
	ErrCreatePersonaTxsNotProcessed = errors.New("create persona txs have not been processed for the given tick")
)

// GetSignerForPersonaTag returns the signer address that has been registered for the given persona tag after the
// given tick. If the engine's tick is less than or equal to the given tick, ErrorCreatePersonaTXsNotProcessed is
// returned. If the given personaTag has no signer address, ErrPersonaTagHasNoSigner is returned.
func (w *World) GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error) {
	if tick >= w.CurrentTick() {
		return "", ErrCreatePersonaTxsNotProcessed
	}
	var errs []error
	wCtx := NewReadOnlyWorldContext(w)
	s := NewSearch(wCtx, filter.Exact(component.SignerComponent{}))
	err = s.Each(
		func(id types.EntityID) bool {
			sc, err := GetComponent[component.SignerComponent](wCtx, id)
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
