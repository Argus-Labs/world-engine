package component

type Reloader struct {
	AmmoCapacity    int
	AmmoQuantity    int
	ChamberCapacity int
	ChamberQuantity int
	TicksPerReload  uint64
	NextReloadTick  uint64
}

func (Reloader) Name() string {
	return "Reloader"
}
