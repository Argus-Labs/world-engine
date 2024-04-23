package component

type Radius struct {
	Length float64
}

func (Radius) Name() string {
	return "Radius"
}
