package component

type Player struct {
	PersonaTag string `json:"personaTag"`
}

func (Player) Name() string {
	return "Player"
}
