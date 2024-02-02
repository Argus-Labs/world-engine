package comp

type ArrayComp struct {
	Numbers [10000]int
}

func (ArrayComp) Name() string {
	return "ArrayComp"
}
