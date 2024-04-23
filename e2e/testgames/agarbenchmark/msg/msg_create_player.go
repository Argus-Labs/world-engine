package msg

type CreatePlayerMsg struct {
	TargetPersonaTag string `json:"targetPersonaTag"`
}

type CreatePlayerResult struct {
	Success bool `json:"success"`
}
