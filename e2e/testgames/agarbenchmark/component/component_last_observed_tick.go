package component

type LastObservedTick struct {
	Tick uint64
}

func (LastObservedTick) Name() string {
	return "LastObservedTick"
}
