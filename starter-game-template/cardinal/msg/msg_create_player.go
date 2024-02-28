package msg

type CreatePlayerMsg struct {
	Nickname string `json:"nickname"`
}

type CreatePlayerResult struct {
	Success bool `json:"success"`
}
