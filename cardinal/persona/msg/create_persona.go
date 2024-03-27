package msg

var CreatePersonaMessageName = "create-persona"

// CreatePersona allows for the associating of a persona tag with a signer address.
type CreatePersona struct {
	PersonaTag    string `json:"personaTag"`
	SignerAddress string `json:"signerAddress"`
}

type CreatePersonaResult struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
}
