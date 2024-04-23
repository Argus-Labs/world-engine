package component

type Offense struct {
	Damage         int
	Range          float64
	TicksPerAttack uint64
	NextAttackTick uint64
}

func (Offense) Name() string {
	return "Offense"
}
