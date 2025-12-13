package testutils

// -------------------------------------------------------------------------------------------------
// Components
// -------------------------------------------------------------------------------------------------

type SimpleComponent struct {
	Value int
}

func (SimpleComponent) Name() string {
	return "simple_component"
}

type ComponentA struct {
	X, Y, Z float64
}

func (ComponentA) Name() string {
	return "component_a"
}

type ComponentB struct {
	ID      uint64
	Label   string
	Enabled bool
}

func (ComponentB) Name() string {
	return "component_b"
}

type ComponentC struct {
	Values  [8]int32
	Counter uint16
}

func (ComponentC) Name() string {
	return "component_c"
}

// -------------------------------------------------------------------------------------------------
// System events
// -------------------------------------------------------------------------------------------------

type SystemEventA struct {
	X, Y, Z float64
}

func (SystemEventA) Name() string {
	return "system_event_a"
}

type SystemEventB struct {
	ID      uint64
	Label   string
	Enabled bool
}

func (SystemEventB) Name() string {
	return "system_event_b"
}

type SystemEventC struct {
	Values  [8]int32
	Counter uint16
}

func (SystemEventC) Name() string {
	return "system_event_c"
}
