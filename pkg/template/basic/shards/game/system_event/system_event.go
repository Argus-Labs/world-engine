package systemevent

type PlayerDeath struct {
	Nickname string
}

func (PlayerDeath) Name() string {
	return "player-death"
}
