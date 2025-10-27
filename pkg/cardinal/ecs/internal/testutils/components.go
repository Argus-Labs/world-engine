package testutils

type Health struct {
	Value int `json:"value"`
}

func (Health) Name() string { return "Health" }

type Position struct{ X, Y int }

func (Position) Name() string { return "Position" }

type Velocity struct{ X, Y int }

func (Velocity) Name() string { return "Velocity" }

type Experience struct{ Value int }

func (Experience) Name() string { return "Experience" }

type PlayerTag struct{ Tag string }

func (PlayerTag) Name() string { return "PlayerTag" }

type Level struct{ Value int }

func (Level) Name() string { return "Level" }

type MapComponent struct {
	Items map[string]int `json:"items"`
}

func (MapComponent) Name() string { return "MapComponent" }
