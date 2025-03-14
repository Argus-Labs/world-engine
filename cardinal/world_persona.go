package cardinal

import (
	"errors"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/persona"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/types"
)

// implements the validator.SignerAddressProvider interface.
func (w *World) GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error) {
	if tick >= w.CurrentTick() {
		return "", persona.ErrCreatePersonaTxsNotProcessed
	}
	var errs []error
	wCtx := NewReadOnlyWorldContext(w)
	s := NewSearch().Entity(filter.Exact(filter.Component[component.SignerComponent]()))
	err = s.Each(wCtx,
		func(id types.EntityID) bool {
			sc, err := GetComponent[component.SignerComponent](wCtx, id)
			if err != nil {
				errs = append(errs, err)
			}
			if sc != nil && sc.PersonaTag == personaTag {
				addr = sc.SignerAddress
				return false
			}
			return true
		},
	)
	errs = append(errs, err)
	if addr == "" {
		return "", persona.ErrPersonaTagHasNoSigner
	}
	return addr, errors.Join(errs...)
}

func (w *World) GetSignerComponentForPersona(personaTag string) (*component.SignerComponent, error) {
	var sc *component.SignerComponent
	wCtx := NewReadOnlyWorldContext(w)
	q := NewSearch().Entity(filter.Exact(filter.Component[component.SignerComponent]()))
	var getComponentErr error
	searchIterationErr := eris.Wrap(
		q.Each(wCtx,
			func(id types.EntityID) bool {
				var signerComp *component.SignerComponent
				signerComp, getComponentErr = GetComponent[component.SignerComponent](wCtx, id)
				if getComponentErr != nil {
					return false
				}
				if signerComp.PersonaTag == personaTag {
					sc = signerComp
					return false
				}
				return true
			},
		), "",
	)
	if getComponentErr != nil {
		return nil, getComponentErr
	}
	if searchIterationErr != nil {
		return nil, searchIterationErr
	}
	if sc == nil {
		return nil, eris.Errorf("persona tag %q not found", personaTag)
	}
	return sc, nil
}
