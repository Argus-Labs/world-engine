package utils

import (
	"errors"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/search"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"regexp"
	"strings"
)

type PersonaTagComponentData struct {
	SignerAddress string
	EntityID      entity.ID
}

// IsValidPersonaTag checks that string is a valid persona tag: alphanumeric + underscore
func IsValidPersonaTag(s string) bool {
	var regexpObj = regexp.MustCompile("^[a-zA-Z0-9_]+$")
	return regexpObj.MatchString(s)
}

func BuildPersonaTagMapping(eCtx engine.Context) (map[string]PersonaTagComponentData, error) {
	personaTagToAddress := map[string]PersonaTagComponentData{}
	var errs []error
	s := search.NewSearch(filter.Exact(component.SignerComponent{}), eCtx.Namespace(), eCtx.StoreReader())
	err := s.Each(
		func(id entity.ID) bool {
			sc, err := ecs.GetComponent[component.SignerComponent](eCtx, id)
			if err != nil {
				errs = append(errs, err)
				return true
			}
			lowerPersona := strings.ToLower(sc.PersonaTag)
			personaTagToAddress[lowerPersona] = PersonaTagComponentData{
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
