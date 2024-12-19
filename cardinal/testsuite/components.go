package testsuite

import (
	"encoding/json"
	"errors"

	"github.com/invopop/jsonschema"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types"
)

// BaseComponent provides common implementation for test components
type BaseComponent struct {
	id types.ComponentID
}

func (b *BaseComponent) SetID(id types.ComponentID) error {
	b.id = id
	return nil
}

func (b *BaseComponent) ID() types.ComponentID {
	return b.id
}

func (b *BaseComponent) Encode(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (b *BaseComponent) GetSchema() []byte {
	schema := jsonschema.Reflect(b)
	bytes, err := schema.MarshalJSON()
	if err != nil {
		return nil
	}
	return bytes
}

func (b *BaseComponent) ValidateAgainstSchema(targetSchema []byte) error {
	if len(targetSchema) == 0 {
		return errors.New("target schema is empty")
	}
	return nil
}

// LocationComponent is a test component for location-based tests
type LocationComponent struct {
	BaseComponent
	X, Y uint64
}

func (l LocationComponent) Name() string {
	return "location"
}

func (l *LocationComponent) New() ([]byte, error) {
	return json.Marshal(&LocationComponent{})
}

func (l *LocationComponent) Decode(data []byte) (types.Component, error) {
	var comp LocationComponent
	if err := json.Unmarshal(data, &comp); err != nil {
		return nil, err
	}
	return &comp, nil
}

// ValueComponent is a test component for value-based tests
type ValueComponent struct {
	BaseComponent
	Value int64
}

func (v ValueComponent) Name() string {
	return "value"
}

func (v *ValueComponent) New() ([]byte, error) {
	return json.Marshal(&ValueComponent{})
}

func (v *ValueComponent) Decode(data []byte) (types.Component, error) {
	var comp ValueComponent
	if err := json.Unmarshal(data, &comp); err != nil {
		return nil, err
	}
	return &comp, nil
}

// PowerComponent is a test component for power-based tests
type PowerComponent struct {
	BaseComponent
	Power int64
}

func (p PowerComponent) Name() string {
	return "power"
}

func (p *PowerComponent) New() ([]byte, error) {
	return json.Marshal(&PowerComponent{})
}

func (p *PowerComponent) Decode(data []byte) (types.Component, error) {
	var comp PowerComponent
	if err := json.Unmarshal(data, &comp); err != nil {
		return nil, err
	}
	return &comp, nil
}

// HealthComponent is a test component for health-based tests
type HealthComponent struct {
	BaseComponent
	Health int64
}

func (h HealthComponent) Name() string {
	return "health"
}

func (h *HealthComponent) New() ([]byte, error) {
	return json.Marshal(&HealthComponent{})
}

func (h *HealthComponent) Decode(data []byte) (types.Component, error) {
	var comp HealthComponent
	if err := json.Unmarshal(data, &comp); err != nil {
		return nil, err
	}
	return &comp, nil
}

// SpeedComponent is a test component for speed-based tests
type SpeedComponent struct {
	BaseComponent
	Speed int64
}

func (s SpeedComponent) Name() string {
	return "speed"
}

func (s *SpeedComponent) New() ([]byte, error) {
	return json.Marshal(&SpeedComponent{})
}

func (s *SpeedComponent) Decode(data []byte) (types.Component, error) {
	var comp SpeedComponent
	if err := json.Unmarshal(data, &comp); err != nil {
		return nil, err
	}
	return &comp, nil
}

// TestComponent is a test component
type TestComponent struct {
	BaseComponent
	Test string
}

func (t TestComponent) Name() string {
	return "test"
}

func (t *TestComponent) New() ([]byte, error) {
	return json.Marshal(&TestComponent{})
}

func (t *TestComponent) Decode(data []byte) (types.Component, error) {
	var comp TestComponent
	if err := json.Unmarshal(data, &comp); err != nil {
		return nil, err
	}
	return &comp, nil
}

// TestTwoComponent is another test component
type TestTwoComponent struct {
	BaseComponent
	TestTwo string
}

func (t TestTwoComponent) Name() string {
	return "test_two"
}

func (t *TestTwoComponent) New() ([]byte, error) {
	return json.Marshal(&TestTwoComponent{})
}

func (t *TestTwoComponent) Decode(data []byte) (types.Component, error) {
	var comp TestTwoComponent
	if err := json.Unmarshal(data, &comp); err != nil {
		return nil, err
	}
	return &comp, nil
}

// RegisterComponents registers all test components with the given world
func RegisterComponents(w *cardinal.World) {
	components := []types.ComponentMetadata{
		&LocationComponent{},
		&ValueComponent{},
		&PowerComponent{},
		&HealthComponent{},
		&SpeedComponent{},
		&TestComponent{},
		&TestTwoComponent{},
	}

	for _, comp := range components {
		if err := w.RegisterComponent(comp); err != nil {
			panic(err)
		}
	}
}
