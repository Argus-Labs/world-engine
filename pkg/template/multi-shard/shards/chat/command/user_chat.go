package command

type UserChat struct {
	ArgusAuthID   string `json:"argus_auth_id"`
	ArgusAuthName string `json:"argus_auth_name"`
	Message       string `json:"message"`
}

func (UserChat) Name() string {
	return "user-chat"
}
