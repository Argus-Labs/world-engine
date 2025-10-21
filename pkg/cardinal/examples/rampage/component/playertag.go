package component

type PlayerTag struct {
	Nickname string `json:"nickname"`
}

func (PlayerTag) Name() string {
	return "playertag"
}
