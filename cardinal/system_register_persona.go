package cardinal

import (
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/persona/utils"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"strings"
)

// RegisterPersonaSystem is an cardinal.System that will associate persona tags with signature addresses. Each persona tag
// may have at most 1 signer, so additional attempts to register a signer with a persona tag will be ignored.
func RegisterPersonaSystem(eCtx engine.Context) error {
	personaTagToAddress, err := buildPersonaIndex(eCtx)
	if err != nil {
		return err
	}

	CreatePersonaMsg.Each(
		eCtx,
		func(txData TxData[msg.CreatePersona]) (result msg.CreatePersonaResult, err error) {
			txMsg := txData.Msg
			result.Success = false

			if !utils.IsValidPersonaTag(txMsg.PersonaTag) {
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
			id, err := Create(eCtx, component.SignerComponent{})
			if err != nil {
				return result, eris.Wrap(err, "")
			}
			if err = SetComponent[component.SignerComponent](
				eCtx, id, &component.SignerComponent{
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

	return nil
}
