package command

type PlayerLeave struct {
	ArgusAuthID string `json:"argus_auth_id"`
}

func (p PlayerLeave) Name() string {
	return "player-leave"
}
