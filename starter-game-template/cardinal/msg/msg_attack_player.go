package msg

type AttackPlayerMsg struct {
	TargetNickname string `json:"target"`
}

type AttackPlayerMsgReply struct {
	Damage int `json:"damage"`
}
