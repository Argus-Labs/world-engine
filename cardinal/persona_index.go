package cardinal

import (
	"errors"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/search"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"strings"
)

type personaIndex = map[string]personaIndexEntry

type personaIndexEntry struct {
	SignerAddress string
	EntityID      entity.ID
}

func buildPersonaIndex(eCtx engine.Context) (personaIndex, error) {
	personaTagToAddress := map[string]personaIndexEntry{}
	var errs []error
	s := search.NewSearch(filter.Exact(component.SignerComponent{}), eCtx.Namespace(), eCtx.StoreReader())
	err := s.Each(
		func(id entity.ID) bool {
			sc, err := GetComponent[component.SignerComponent](eCtx, id)
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
