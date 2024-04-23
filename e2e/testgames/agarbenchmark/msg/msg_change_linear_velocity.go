package msg

type ChangeLinearVelocityMsg struct {
	LinearVelocityX float64 `json:"x"`
	LinearVelocityY float64 `json:"y"`
}

type ChangeLinearVelocityResult struct {
	Success bool `json:"success"`
}
