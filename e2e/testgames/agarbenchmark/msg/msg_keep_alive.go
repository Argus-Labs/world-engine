package msg

type KeepAliveMsg struct {
}

type KeepAliveResult struct {
	Success bool `json:"success"`
}
