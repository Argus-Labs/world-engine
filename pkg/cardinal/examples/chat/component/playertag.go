package component

type UserTag struct {
	ArgusAuthID   string `json:"argus_auth_id"`
	ArgusAuthName string `json:"argus_auth_name"`
}

func (UserTag) Name() string {
	return "usertag"
}
