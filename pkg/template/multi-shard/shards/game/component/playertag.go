package component

type PlayerTag struct {
	ArgusAuthID   string `json:"argus_auth_id"`
	ArgusAuthName string `json:"argus_auth_name"`
}

func (PlayerTag) Name() string {
	return "playertag"
}
