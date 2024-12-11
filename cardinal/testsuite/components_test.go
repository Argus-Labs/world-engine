package testsuite

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocationComponent(t *testing.T) {
	tests := []struct {
		name     string
		loc      LocationComponent
		wantX    uint64
		wantY    uint64
		wantName string
	}{
		{
			name:     "returns correct component name",
			loc:      LocationComponent{},
			wantX:    0,
			wantY:    0,
			wantName: "location",
		},
		{
			name: "stores positive coordinates",
			loc: LocationComponent{
				X: 10,
				Y: 20,
			},
			wantX:    10,
			wantY:    20,
			wantName: "location",
		},
		{
			name: "handles large coordinates",
			loc: LocationComponent{
				X: 999999,
				Y: 888888,
			},
			wantX:    999999,
			wantY:    888888,
			wantName: "location",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantName, tt.loc.Name(), "component name should match")
			assert.Equal(t, tt.wantX, tt.loc.X, "X coordinate should match")
			assert.Equal(t, tt.wantY, tt.loc.Y, "Y coordinate should match")
		})
	}
}

func TestValueComponent(t *testing.T) {
	tests := []struct {
		name      string
		component ValueComponent
		value     int64
		wantName  string
	}{
		{
			name:      "returns correct component name",
			component: ValueComponent{},
			wantName:  "value",
		},
		{
			name: "stores positive value",
			component: ValueComponent{
				Value: 100,
			},
			value:    100,
			wantName: "value",
		},
		{
			name: "stores negative value",
			component: ValueComponent{
				Value: -50,
			},
			value:    -50,
			wantName: "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantName, tt.component.Name(), "component name should match")
			assert.Equal(t, tt.value, tt.component.Value, "component value should match")
		})
	}
}

func TestPowerComponent(t *testing.T) {
	tests := []struct {
		name      string
		component PowerComponent
		power     int64
		wantName  string
	}{
		{
			name:      "returns correct component name",
			component: PowerComponent{},
			wantName:  "power",
		},
		{
			name: "stores positive power value",
			component: PowerComponent{
				Power: 1000,
			},
			power:    1000,
			wantName: "power",
		},
		{
			name: "stores negative power value",
			component: PowerComponent{
				Power: -500,
			},
			power:    -500,
			wantName: "power",
		},
		{
			name: "stores zero power value",
			component: PowerComponent{
				Power: 0,
			},
			power:    0,
			wantName: "power",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantName, tt.component.Name(), "component name should match")
			assert.Equal(t, tt.power, tt.component.Power, "component power should match")
		})
	}
}
