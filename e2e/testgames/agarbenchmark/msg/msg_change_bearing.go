package msg

type ChangeBearingMsg struct {
	Bearing float64 `json:"bearing"`
}

type ChangeBearingResult struct {
	Success bool `json:"success"`
}
