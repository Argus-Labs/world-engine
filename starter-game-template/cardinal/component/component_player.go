package component

type Player struct {
	Nickname string `json:"nickname"`
}

func (Player) Name() string {
	return "Player"
}
