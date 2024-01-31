package comp

type Player struct {
	ID string `json:"Name"`
}

func (p Player) Name() string {
	return "player"
}
