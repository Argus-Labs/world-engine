package component

type Gravestone struct {
	Nickname string `json:"nickname"`
}

func (Gravestone) Name() string {
	return "gravestone"
}
