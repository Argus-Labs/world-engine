package msg

type AuthorizePersonaAddress struct {
	Address string `json:"address"`
}

type AuthorizePersonaAddressResult struct {
	Success bool `json:"success"`
}
