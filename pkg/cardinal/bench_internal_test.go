package cardinal

import (
	"bytes"
	"encoding/gob"

	"reflect"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
)

// Bench component fixtures satisfy schema.Serializable via gob (real components get generated proto
// codecs; these doubles only need to round-trip). The ENCODE side is testutils.GobMarshal and
// friends, shared so a fixture's SizeWire, AppendWire and MarshalWire cannot disagree. Only the
// decode side is local, because it returns any — a decode factory — rather than T.
func benchGobUnmarshal[T any](b []byte) (any, error) {
	var v T
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func (c Position3D) SizeWire() int               { return len(c.MarshalWire()) }
func (c Position3D) AppendWire(b []byte) []byte  { return append(b, c.MarshalWire()...) }
func (c Velocity3D) SizeWire() int               { return len(c.MarshalWire()) }
func (c Velocity3D) AppendWire(b []byte) []byte  { return append(b, c.MarshalWire()...) }
func (c Health2) SizeWire() int                  { return len(c.MarshalWire()) }
func (c Health2) AppendWire(b []byte) []byte     { return append(b, c.MarshalWire()...) }
func (c Transform) SizeWire() int                { return len(c.MarshalWire()) }
func (c Transform) AppendWire(b []byte) []byte   { return append(b, c.MarshalWire()...) }
func (c Inventory) SizeWire() int                { return len(c.MarshalWire()) }
func (c Inventory) AppendWire(b []byte) []byte   { return append(b, c.MarshalWire()...) }
func (c PlayerStats) SizeWire() int              { return len(c.MarshalWire()) }
func (c PlayerStats) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }
func (c AIBehavior) SizeWire() int               { return len(c.MarshalWire()) }
func (c AIBehavior) AppendWire(b []byte) []byte  { return append(b, c.MarshalWire()...) }
func (c Renderer) SizeWire() int                 { return len(c.MarshalWire()) }
func (c Renderer) AppendWire(b []byte) []byte    { return append(b, c.MarshalWire()...) }
func (c Physics) SizeWire() int                  { return len(c.MarshalWire()) }
func (c Physics) AppendWire(b []byte) []byte     { return append(b, c.MarshalWire()...) }
func (c NetworkSync) SizeWire() int              { return len(c.MarshalWire()) }
func (c NetworkSync) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

func (c Position3D) MarshalWire() []byte                { return testutils.GobMarshal(c) }
func (Position3D) UnmarshalWire(b []byte) (any, error)  { return benchGobUnmarshal[Position3D](b) }
func (c Velocity3D) MarshalWire() []byte                { return testutils.GobMarshal(c) }
func (Velocity3D) UnmarshalWire(b []byte) (any, error)  { return benchGobUnmarshal[Velocity3D](b) }
func (c Health2) MarshalWire() []byte                   { return testutils.GobMarshal(c) }
func (Health2) UnmarshalWire(b []byte) (any, error)     { return benchGobUnmarshal[Health2](b) }
func (c Transform) MarshalWire() []byte                 { return testutils.GobMarshal(c) }
func (Transform) UnmarshalWire(b []byte) (any, error)   { return benchGobUnmarshal[Transform](b) }
func (c Inventory) MarshalWire() []byte                 { return testutils.GobMarshal(c) }
func (Inventory) UnmarshalWire(b []byte) (any, error)   { return benchGobUnmarshal[Inventory](b) }
func (c PlayerStats) MarshalWire() []byte               { return testutils.GobMarshal(c) }
func (PlayerStats) UnmarshalWire(b []byte) (any, error) { return benchGobUnmarshal[PlayerStats](b) }
func (c AIBehavior) MarshalWire() []byte                { return testutils.GobMarshal(c) }
func (AIBehavior) UnmarshalWire(b []byte) (any, error)  { return benchGobUnmarshal[AIBehavior](b) }
func (c Renderer) MarshalWire() []byte                  { return testutils.GobMarshal(c) }
func (Renderer) UnmarshalWire(b []byte) (any, error)    { return benchGobUnmarshal[Renderer](b) }
func (c Physics) MarshalWire() []byte                   { return testutils.GobMarshal(c) }
func (Physics) UnmarshalWire(b []byte) (any, error)     { return benchGobUnmarshal[Physics](b) }
func (c NetworkSync) MarshalWire() []byte               { return testutils.GobMarshal(c) }
func (NetworkSync) UnmarshalWire(b []byte) (any, error) { return benchGobUnmarshal[NetworkSync](b) }

type entityState1 struct {
	Entities Contains[struct{ Position WithComponent[Position3D] }]
}

type entityState2 struct {
	Entities Contains[struct {
		Position WithComponent[Position3D]
		Velocity WithComponent[Velocity3D]
	}]
}

type entityState5 struct {
	Entities Contains[struct {
		Position  WithComponent[Position3D]
		Velocity  WithComponent[Velocity3D]
		Health    WithComponent[Health2]
		Transform WithComponent[Transform]
		Inventory WithComponent[Inventory]
	}]
}

type entityState10 struct {
	Entities Contains[struct {
		Position    WithComponent[Position3D]
		Velocity    WithComponent[Velocity3D]
		Health      WithComponent[Health2]
		Transform   WithComponent[Transform]
		Inventory   WithComponent[Inventory]
		PlayerStats WithComponent[PlayerStats]
		AIBehavior  WithComponent[AIBehavior]
		Renderer    WithComponent[Renderer]
		Physics     WithComponent[Physics]
		NetworkSync WithComponent[NetworkSync]
	}]
}

func BenchmarkCardinal_Entity_Create(b *testing.B) {
	b.Run("1 component with archetype creation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState1{}
			mustInitSystemFields(b, w, state)

			b.StartTimer()
			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			b.StopTimer()
		}
	})

	b.Run("1 component existing archetype", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState1{}
			mustInitSystemFields(b, w, state)

			warmup := state.Entities.Create()
			warmup.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			b.StopTimer()
		}
	})

	b.Run("5 components with archetype creation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState5{}
			mustInitSystemFields(b, w, state)

			b.StartTimer()
			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Set(Health2{Current: 100, Max: 100})
			entity.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			b.StopTimer()
		}
	})

	b.Run("5 components existing archetype", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState5{}
			mustInitSystemFields(b, w, state)

			warmup := state.Entities.Create()
			warmup.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			warmup.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			warmup.Set(Health2{Current: 100, Max: 100})
			warmup.Set(Transform{Scale: 1.0, Rotation: 0.0})
			warmup.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})

			b.StartTimer()
			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Set(Health2{Current: 100, Max: 100})
			entity.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			b.StopTimer()
		}
	})

	b.Run("10 components with archetype creation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState10{}
			mustInitSystemFields(b, w, state)

			b.StartTimer()
			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Set(Health2{Current: 100, Max: 100})
			entity.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			entity.Set(PlayerStats{Level: 5, Experience: 1000, Strength: 10, Agility: 8})
			entity.Set(AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0})
			entity.Set(Renderer{Model: "player", Texture: "player.png", Visible: true, ZIndex: 1})
			entity.Set(Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false})
			entity.Set(
				NetworkSync{PlayerID: "player1", LastUpdate: 0, SyncRate: 30.0, IsDirty: false, Interpolate: true})
			b.StopTimer()
		}
	})

	b.Run("10 components existing archetype", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState10{}
			mustInitSystemFields(b, w, state)

			warmup := state.Entities.Create()
			warmup.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			warmup.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			warmup.Set(Health2{Current: 100, Max: 100})
			warmup.Set(Transform{Scale: 1.0, Rotation: 0.0})
			warmup.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			warmup.Set(PlayerStats{Level: 5, Experience: 1000, Strength: 10, Agility: 8})
			warmup.Set(AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0})
			warmup.Set(Renderer{Model: "player", Texture: "player.png", Visible: true, ZIndex: 1})
			warmup.Set(Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false})
			warmup.Set(
				NetworkSync{PlayerID: "player1", LastUpdate: 0, SyncRate: 30.0, IsDirty: false, Interpolate: true})

			b.StartTimer()
			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Set(Health2{Current: 100, Max: 100})
			entity.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			entity.Set(PlayerStats{Level: 5, Experience: 1000, Strength: 10, Agility: 8})
			entity.Set(AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0})
			entity.Set(Renderer{Model: "player", Texture: "player.png", Visible: true, ZIndex: 1})
			entity.Set(Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false})
			entity.Set(
				NetworkSync{PlayerID: "player1", LastUpdate: 0, SyncRate: 30.0, IsDirty: false, Interpolate: true})
			b.StopTimer()
		}
	})
}

func BenchmarkCardinal_Entity_Destroy(b *testing.B) {
	b.Run("1 component", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState1{}
			mustInitSystemFields(b, w, state)

			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			_ = entity.Destroy()
			b.StopTimer()
		}
	})

	b.Run("5 components", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState5{}
			mustInitSystemFields(b, w, state)

			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Set(Health2{Current: 100, Max: 100})
			entity.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})

			b.StartTimer()
			_ = entity.Destroy()
			b.StopTimer()
		}
	})

	b.Run("10 components", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState10{}
			mustInitSystemFields(b, w, state)

			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Set(Health2{Current: 100, Max: 100})
			entity.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			entity.Set(PlayerStats{Level: 5, Experience: 1000, Strength: 10, Agility: 8})
			entity.Set(AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0})
			entity.Set(Renderer{Model: "player", Texture: "player.png", Visible: true, ZIndex: 1})
			entity.Set(Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false})
			entity.Set(
				NetworkSync{PlayerID: "player1", LastUpdate: 0, SyncRate: 30.0, IsDirty: false, Interpolate: true})

			b.StartTimer()
			_ = entity.Destroy()
			b.StopTimer()
		}
	})
}

func BenchmarkCardinal_Component_Set(b *testing.B) {
	b.Run("update existing component", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState2{}
			mustInitSystemFields(b, w, state)

			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})

			b.StartTimer()
			entity.Set(Position3D{X: 10.0, Y: 20.0, Z: 30.0})
			b.StopTimer()
		}
	})

	b.Run("add new component", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &struct {
				PositionOnly Contains[struct{ Position WithComponent[Position3D] }]
				PositionHP   Contains[struct {
					Position WithComponent[Position3D]
					Health   WithComponent[Health2]
				}]
			}{}
			mustInitSystemFields(b, w, state)

			entity := state.PositionOnly.Create()

			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			entity.Set(Health2{Current: 100, Max: 100})
			b.StopTimer()
		}
	})
}

func BenchmarkCardinal_Component_Remove(b *testing.B) {
	b.Run("remove last component (delete entity)", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState1{}
			mustInitSystemFields(b, w, state)

			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			entity.Remove[Position3D]()
			b.StopTimer()
		}
	})

	b.Run("remove component from 5-component entity", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState5{}
			mustInitSystemFields(b, w, state)

			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Set(Health2{Current: 100, Max: 100})
			entity.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Set(Inventory{Items: []string{"sword"}, Capacity: 10})

			b.StartTimer()
			entity.Remove[Velocity3D]()
			b.StopTimer()
		}
	})
}

func BenchmarkCardinal_Component_Get(b *testing.B) {
	b.Run("get component from 1-component entity", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState1{}
			mustInitSystemFields(b, w, state)

			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			_ = entity.Get[Position3D]()
			b.StopTimer()
		}
	})

	b.Run("get component from 5-component entity", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState5{}
			mustInitSystemFields(b, w, state)

			entity := state.Entities.Create()
			entity.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Set(Health2{Current: 100, Max: 100})
			entity.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Set(Inventory{Items: []string{"sword"}, Capacity: 10})

			b.StartTimer()
			_ = entity.Get[Position3D]()
			b.StopTimer()
		}
	})
}

func BenchmarkCardinal_Iteration_Pure(b *testing.B) {
	b.Run("Exact/1 component", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &struct {
				Creator Contains[struct{ Position WithComponent[Position3D] }]
				Query   Exact[struct{ Position WithComponent[Position3D] }]
			}{}
			mustInitSystemFields(b, w, state)

			for j := 0; j < 100; j++ {
				entity := state.Creator.Create()
				entity.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
			}

			b.StartTimer()
			for result := range state.Query.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})

	b.Run("Exact/5 components", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &struct {
				Creator Contains[struct {
					Position  WithComponent[Position3D]
					Velocity  WithComponent[Velocity3D]
					Health    WithComponent[Health2]
					Transform WithComponent[Transform]
					Inventory WithComponent[Inventory]
				}]
				Query Exact[struct {
					Position  WithComponent[Position3D]
					Velocity  WithComponent[Velocity3D]
					Health    WithComponent[Health2]
					Transform WithComponent[Transform]
					Inventory WithComponent[Inventory]
				}]
			}{}
			mustInitSystemFields(b, w, state)

			for j := 0; j < 100; j++ {
				entity := state.Creator.Create()
				entity.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
				entity.Set(Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)})
				entity.Set(Health2{Current: j, Max: 100})
				entity.Set(Transform{Scale: 1.0, Rotation: float64(j)})
				entity.Set(Inventory{Items: []string{"item"}, Capacity: 10})
			}

			b.StartTimer()
			for result := range state.Query.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})

	b.Run("Exact/10 components", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &struct {
				Creator Contains[struct {
					Position    WithComponent[Position3D]
					Velocity    WithComponent[Velocity3D]
					Health      WithComponent[Health2]
					Transform   WithComponent[Transform]
					Inventory   WithComponent[Inventory]
					PlayerStats WithComponent[PlayerStats]
					AIBehavior  WithComponent[AIBehavior]
					Renderer    WithComponent[Renderer]
					Physics     WithComponent[Physics]
					NetworkSync WithComponent[NetworkSync]
				}]
				Query Exact[struct {
					Position    WithComponent[Position3D]
					Velocity    WithComponent[Velocity3D]
					Health      WithComponent[Health2]
					Transform   WithComponent[Transform]
					Inventory   WithComponent[Inventory]
					PlayerStats WithComponent[PlayerStats]
					AIBehavior  WithComponent[AIBehavior]
					Renderer    WithComponent[Renderer]
					Physics     WithComponent[Physics]
					NetworkSync WithComponent[NetworkSync]
				}]
			}{}
			mustInitSystemFields(b, w, state)

			for j := 0; j < 100; j++ {
				entity := state.Creator.Create()
				entity.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
				entity.Set(Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)})
				entity.Set(Health2{Current: j, Max: 100})
				entity.Set(Transform{Scale: 1.0, Rotation: float64(j)})
				entity.Set(Inventory{Items: []string{"item"}, Capacity: 10})
				entity.Set(PlayerStats{Level: j, Experience: j * 10, Strength: 10, Agility: 8})
				entity.Set(AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0})
				entity.Set(Renderer{Model: "model", Texture: "texture", Visible: true, ZIndex: 1})
				entity.Set(Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false})
				entity.Set(NetworkSync{
					PlayerID: "player", LastUpdate: int64(j), SyncRate: 30.0,
					IsDirty: false, Interpolate: true,
				})
			}

			b.StartTimer()
			for result := range state.Query.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})

	b.Run("Contains/1 from 1 component entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &struct {
				Creator Contains[struct{ Position WithComponent[Position3D] }]
				Query   Contains[struct{ Position WithComponent[Position3D] }]
			}{}
			mustInitSystemFields(b, w, state)

			for j := 0; j < 100; j++ {
				entity := state.Creator.Create()
				entity.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
			}

			b.StartTimer()
			for result := range state.Query.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})

	b.Run("Contains/3 from 5 component entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &struct {
				Creator Contains[struct {
					Position  WithComponent[Position3D]
					Velocity  WithComponent[Velocity3D]
					Health    WithComponent[Health2]
					Transform WithComponent[Transform]
					Inventory WithComponent[Inventory]
				}]
				Query Contains[struct {
					Position WithComponent[Position3D]
					Velocity WithComponent[Velocity3D]
					Health   WithComponent[Health2]
				}]
			}{}
			mustInitSystemFields(b, w, state)

			for j := 0; j < 100; j++ {
				entity := state.Creator.Create()
				entity.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
				entity.Set(Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)})
				entity.Set(Health2{Current: j, Max: 100})
				entity.Set(Transform{Scale: 1.0, Rotation: float64(j)})
				entity.Set(Inventory{Items: []string{"item"}, Capacity: 10})
			}

			b.StartTimer()
			for result := range state.Query.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})

	b.Run("Contains/6 from 10 component entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &struct {
				Creator Contains[struct {
					Position    WithComponent[Position3D]
					Velocity    WithComponent[Velocity3D]
					Health      WithComponent[Health2]
					Transform   WithComponent[Transform]
					Inventory   WithComponent[Inventory]
					PlayerStats WithComponent[PlayerStats]
					AIBehavior  WithComponent[AIBehavior]
					Renderer    WithComponent[Renderer]
					Physics     WithComponent[Physics]
					NetworkSync WithComponent[NetworkSync]
				}]
				Query Contains[struct {
					Position    WithComponent[Position3D]
					Velocity    WithComponent[Velocity3D]
					Health      WithComponent[Health2]
					Transform   WithComponent[Transform]
					Inventory   WithComponent[Inventory]
					PlayerStats WithComponent[PlayerStats]
				}]
			}{}
			mustInitSystemFields(b, w, state)

			for j := 0; j < 100; j++ {
				entity := state.Creator.Create()
				entity.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
				entity.Set(Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)})
				entity.Set(Health2{Current: j, Max: 100})
				entity.Set(Transform{Scale: 1.0, Rotation: float64(j)})
				entity.Set(Inventory{Items: []string{"item"}, Capacity: 10})
				entity.Set(PlayerStats{Level: j, Experience: j * 10, Strength: 10, Agility: 8})
				entity.Set(AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0})
				entity.Set(Renderer{Model: "model", Texture: "texture", Visible: true, ZIndex: 1})
				entity.Set(Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false})
				entity.Set(NetworkSync{
					PlayerID: "player", LastUpdate: int64(j), SyncRate: 30.0,
					IsDirty: false, Interpolate: true,
				})
			}

			b.StartTimer()
			for result := range state.Query.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})
}

type getSetSystemState1 struct {
	BaseSystemState
	Entities Contains[struct {
		Position WithComponent[Position3D]
	}]
}

type getSetSystemState5 struct {
	BaseSystemState
	Entities Contains[struct {
		Position  WithComponent[Position3D]
		Velocity  WithComponent[Velocity3D]
		Health    WithComponent[Health2]
		Transform WithComponent[Transform]
		Inventory WithComponent[Inventory]
	}]
}

type getSetSystemState10 struct {
	BaseSystemState
	Entities Contains[struct {
		Position    WithComponent[Position3D]
		Velocity    WithComponent[Velocity3D]
		Health      WithComponent[Health2]
		Transform   WithComponent[Transform]
		Inventory   WithComponent[Inventory]
		PlayerStats WithComponent[PlayerStats]
		AIBehavior  WithComponent[AIBehavior]
		Renderer    WithComponent[Renderer]
		Physics     WithComponent[Physics]
		NetworkSync WithComponent[NetworkSync]
	}]
}

func BenchmarkCardinal_Iteration_GetSet(b *testing.B) {
	b.Run("1 component 100 entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()

			w.RegisterSystem(func(state *getSetSystemState1) {
				for j := 0; j < 100; j++ {
					entity := state.Entities.Create()
					entity.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
				}
			}, WithHook(Init))

			w.RegisterSystem(func(state *getSetSystemState1) {
				b.StartTimer()
				for entity := range state.Entities.Iter() {
					pos := entity.Get[Position3D]()
					pos.X += 1.0
					entity.Set(pos)
				}
				b.StopTimer()
			}, WithHook(Update))

			w.world.Init()
			w.world.Tick()
			w.world.Tick()
		}
	})

	b.Run("5 components 100 entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()

			w.RegisterSystem(func(state *getSetSystemState5) {
				for j := 0; j < 100; j++ {
					entity := state.Entities.Create()
					entity.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
					entity.Set(Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)})
					entity.Set(Health2{Current: j, Max: 100})
					entity.Set(Transform{Scale: 1.0, Rotation: float64(j)})
					entity.Set(Inventory{Items: []string{"item"}, Capacity: 10})
				}
			}, WithHook(Init))

			w.RegisterSystem(func(state *getSetSystemState5) {
				b.StartTimer()
				for entity := range state.Entities.Iter() {
					pos := entity.Get[Position3D]()
					vel := entity.Get[Velocity3D]()
					vel.X = pos.X * 0.1
					vel.Y = pos.Y * 0.1
					entity.Set(vel)
				}
				b.StopTimer()
			}, WithHook(Update))

			w.world.Init()
			w.world.Tick()
			w.world.Tick()
		}
	})

	b.Run("10 components 100 entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()

			w.RegisterSystem(func(state *getSetSystemState10) {
				for j := 0; j < 100; j++ {
					entity := state.Entities.Create()
					entity.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
					entity.Set(Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)})
					entity.Set(Health2{Current: j, Max: 100})
					entity.Set(Transform{Scale: 1.0, Rotation: float64(j)})
					entity.Set(Inventory{Items: []string{"item"}, Capacity: 10})
					entity.Set(PlayerStats{Level: j, Experience: j * 10, Strength: 10, Agility: 8})
					entity.Set(AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0})
					entity.Set(Renderer{Model: "model", Texture: "texture", Visible: true, ZIndex: 1})
					entity.Set(Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false})
					entity.Set(NetworkSync{
						PlayerID: "player", LastUpdate: int64(j), SyncRate: 30.0,
						IsDirty: false, Interpolate: true,
					})
				}
			}, WithHook(Init))

			w.RegisterSystem(func(state *getSetSystemState10) {
				b.StartTimer()
				for entity := range state.Entities.Iter() {
					pos := entity.Get[Position3D]()
					health := entity.Get[Health2]()

					physics := entity.Get[Physics]()
					physics.Mass = pos.X * 0.01
					entity.Set(physics)

					renderer := entity.Get[Renderer]()
					renderer.Visible = health.Current > 50
					entity.Set(renderer)
				}
				b.StopTimer()
			}, WithHook(Update))

			w.world.Init()
			w.world.Tick()
			w.world.Tick()
		}
	})
}

func newBenchWorld() *World {
	w := &World{world: ecs.NewWorld()}
	w.RegisterComponent[Position3D]()
	w.RegisterComponent[Velocity3D]()
	w.RegisterComponent[Health2]()
	w.RegisterComponent[Inventory]()
	w.RegisterComponent[Transform]()
	w.RegisterComponent[PlayerStats]()
	w.RegisterComponent[AIBehavior]()
	w.RegisterComponent[NetworkSync]()
	w.RegisterComponent[Physics]()
	w.RegisterComponent[Renderer]()
	return w
}

func mustInitSystemFields[T any](b testing.TB, w *World, state *T) {
	b.Helper()
	err := initSystemFields(reflect.ValueOf(state).Elem(), w)
	if err != nil {
		b.Fatalf("failed to initialize system fields: %v", err)
	}
}

// Benchmark component types.
type Position3D struct {
	X, Y, Z float64
}

func (Position3D) Name() string { return "Position" }

type Velocity3D struct {
	X, Y, Z float64
}

func (Velocity3D) Name() string { return "Velocity" }

type Health2 struct {
	Current, Max int
}

func (Health2) Name() string { return "Health" }

type Transform struct {
	Scale    float64
	Rotation float64
}

func (Transform) Name() string { return "Transform" }

type Inventory struct {
	Items    []string
	Capacity int
}

func (Inventory) Name() string { return "Inventory" }

type PlayerStats struct {
	Level      int
	Experience int
	Strength   int
	Agility    int
}

func (PlayerStats) Name() string { return "PlayerStats" }

type AIBehavior struct {
	State       string
	Target      EntityID
	Aggression  float64
	PatrolRange float64
}

func (AIBehavior) Name() string { return "AIBehavior" }

type Renderer struct {
	Model   string
	Texture string
	Visible bool
	ZIndex  int
}

func (Renderer) Name() string { return "Renderer" }

type Physics struct {
	Mass        float64
	Friction    float64
	Restitution float64
	IsStatic    bool
}

func (Physics) Name() string { return "Physics" }

type NetworkSync struct {
	PlayerID    string
	LastUpdate  int64
	SyncRate    float64
	IsDirty     bool
	Interpolate bool
}

func (NetworkSync) Name() string { return "NetworkSync" }
