package event

type PlayerDeath struct {
	Nickname string
}

func (PlayerDeath) Name() string {
	return "player-death"
}

type NewPlayer struct {
	Nickname string
}

func (NewPlayer) Name() string {
	return "new-player"
}
