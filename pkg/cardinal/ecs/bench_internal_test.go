package ecs

import (
	"testing"

	"github.com/kelindar/bitmap"
)

// BenchmarkECS2_Entity_Create benchmarks entity creation with varying component counts.
func BenchmarkECS2_Entity_Create(b *testing.B) {
	benchmarks := []struct {
		name       string
		components []Component
		create     func(*worldState) EntityID
	}{
		{
			name:       "1 component",
			components: []Component{Position3D{}},
			create: func(ws *worldState) EntityID {
				arch := Archetype1{}
				return arch.CreateEntities(ws, Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			},
		},
		{
			name:       "5 components",
			components: []Component{Position3D{}, Velocity3D{}, Health2{}, Transform{}, Inventory{}},
			create: func(ws *worldState) EntityID {
				arch := Archetype5{}
				return arch.CreateEntities(ws,
					Position3D{X: 1.0, Y: 2.0, Z: 3.0},
					Velocity3D{X: 0.5, Y: 1.0, Z: -0.2},
					Health2{Current: 100, Max: 100},
					Transform{Scale: 1.0, Rotation: 0.0},
					Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			},
		},
		{
			name: "10 components",
			components: []Component{
				Position3D{}, Velocity3D{}, Health2{}, Transform{}, Inventory{},
				PlayerStats{}, AIBehavior{}, Renderer{}, Physics{}, NetworkSync{},
			},
			create: func(ws *worldState) EntityID {
				arch := Archetype10{}
				return arch.CreateEntities(ws,
					Position3D{X: 1.0, Y: 2.0, Z: 3.0},
					Velocity3D{X: 0.5, Y: 1.0, Z: -0.2},
					Health2{Current: 100, Max: 100},
					Transform{Scale: 1.0, Rotation: 0.0},
					Inventory{Items: []string{"sword", "potion"}, Capacity: 10},
					PlayerStats{Level: 5, Experience: 1000, Strength: 10, Agility: 8},
					AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0},
					Renderer{Model: "player", Texture: "player.png", Visible: true, ZIndex: 1},
					Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false},
					NetworkSync{PlayerID: "player1", LastUpdate: 0, SyncRate: 30.0, IsDirty: false, Interpolate: true})
			},
		},
	}

	for _, bm := range benchmarks {
		// Benchmark full cost including archetype creation
		b.Run(bm.name+" with archetype creation", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w := setup2(bm.components...)
				ws := w.state

				b.StartTimer()
				_ = bm.create(ws)
				b.StopTimer()
			}
		})

		// Benchmark just the entity creation cost when archetype already exists
		b.Run(bm.name+" existing archetype", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w := setup2(bm.components...)
				ws := w.state
				bm.create(ws) // Create one entity to ensure archetype exists

				b.StartTimer()
				_ = bm.create(ws)
				b.StopTimer()
			}
		})
	}
}

// BenchmarkECS2_Entity_Destroy benchmarks entity destruction with varying component counts.
func BenchmarkECS2_Entity_Destroy(b *testing.B) {
	benchmarks := []struct {
		name       string
		components []Component
		create     func(*worldState) EntityID
	}{
		{
			name:       "1 component",
			components: []Component{Position3D{}},
			create: func(ws *worldState) EntityID {
				arch := Archetype1{}
				return arch.CreateEntities(ws, Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			},
		},
		{
			name:       "5 components",
			components: []Component{Position3D{}, Velocity3D{}, Health2{}, Transform{}, Inventory{}},
			create: func(ws *worldState) EntityID {
				arch := Archetype5{}
				return arch.CreateEntities(ws,
					Position3D{X: 1.0, Y: 2.0, Z: 3.0},
					Velocity3D{X: 0.5, Y: 1.0, Z: -0.2},
					Health2{Current: 100, Max: 100},
					Transform{Scale: 1.0, Rotation: 0.0},
					Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			},
		},
		{
			name: "10 components",
			components: []Component{
				Position3D{}, Velocity3D{}, Health2{}, Transform{}, Inventory{},
				PlayerStats{}, AIBehavior{}, Renderer{}, Physics{}, NetworkSync{},
			},
			create: func(ws *worldState) EntityID {
				arch := Archetype10{}
				return arch.CreateEntities(ws,
					Position3D{X: 1.0, Y: 2.0, Z: 3.0},
					Velocity3D{X: 0.5, Y: 1.0, Z: -0.2},
					Health2{Current: 100, Max: 100},
					Transform{Scale: 1.0, Rotation: 0.0},
					Inventory{Items: []string{"sword", "potion"}, Capacity: 10},
					PlayerStats{Level: 5, Experience: 1000, Strength: 10, Agility: 8},
					AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0},
					Renderer{Model: "player", Texture: "player.png", Visible: true, ZIndex: 1},
					Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false},
					NetworkSync{PlayerID: "player1", LastUpdate: 0, SyncRate: 30.0, IsDirty: false, Interpolate: true})
			},
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w := setup2(bm.components...)
				ws := w.state
				var entity = bm.create(ws) // Create entity to destroy

				b.StartTimer()
				Destroy(ws, entity)
				b.StopTimer()
			}
		})
	}
}

// BenchmarkECS2_Component_Set benchmarks component setting operations.
func BenchmarkECS2_Component_Set(b *testing.B) {
	b.Run("update existing component", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(Position3D{}, Velocity3D{})
			ws := w.state
			var entity EntityID
			arch := Archetype2{}
			entity = arch.CreateEntities(ws, Position3D{X: 1.0, Y: 2.0, Z: 3.0}, Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})

			b.StartTimer()
			_ = Set(ws, entity, Position3D{X: 10.0, Y: 20.0, Z: 30.0})
			b.StopTimer()
		}
	})

	b.Run("add new component", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(Position3D{}, Health2{})
			ws := w.state
			var entity EntityID
			arch := Archetype1{}
			entity = arch.CreateEntities(ws, Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			_ = Set(ws, entity, Health2{Current: 100, Max: 100})
			b.StopTimer()
		}
	})
}

// BenchmarkECS2_Component_Remove benchmarks component removal.
func BenchmarkECS2_Component_Remove(b *testing.B) {
	b.Run("remove last component (delete entity)", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(Position3D{})
			ws := w.state
			var entity EntityID
			arch := Archetype1{}
			entity = arch.CreateEntities(ws, Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			_ = Remove[Position3D](ws, entity)
			b.StopTimer()
		}
	})

	b.Run("remove component from 5-component entity", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(Position3D{}, Velocity3D{}, Health2{}, Transform{}, Inventory{})
			ws := w.state
			var entity EntityID
			arch := Archetype5{}
			entity = arch.CreateEntities(ws,
				Position3D{X: 1.0, Y: 2.0, Z: 3.0},
				Velocity3D{X: 0.5, Y: 1.0, Z: -0.2},
				Health2{Current: 100, Max: 100},
				Transform{Scale: 1.0, Rotation: 0.0},
				Inventory{Items: []string{"sword"}, Capacity: 10})

			b.StartTimer()
			_ = Remove[Velocity3D](ws, entity)
			b.StopTimer()
		}
	})
}

// BenchmarkECS2_Component_Get benchmarks component retrieval.
func BenchmarkECS2_Component_Get(b *testing.B) {
	b.Run("get component from 1-component entity", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(Position3D{})
			ws := w.state
			var entity EntityID
			arch := Archetype1{}
			entity = arch.CreateEntities(ws, Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			_, _ = Get[Position3D](ws, entity)
			b.StopTimer()
		}
	})

	b.Run("get component from 5-component entity", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(Position3D{}, Velocity3D{}, Health2{}, Transform{}, Inventory{})
			ws := w.state
			var entity EntityID
			arch := Archetype5{}
			entity = arch.CreateEntities(ws,
				Position3D{X: 1.0, Y: 2.0, Z: 3.0},
				Velocity3D{X: 0.5, Y: 1.0, Z: -0.2},
				Health2{Current: 100, Max: 100},
				Transform{Scale: 1.0, Rotation: 0.0},
				Inventory{Items: []string{"sword"}, Capacity: 10})

			b.StartTimer()
			_, _ = Get[Position3D](ws, entity)
			b.StopTimer()
		}
	})
}

// System state types for iteration benchmarks.
type getSetSystemState1 struct {
	Entities Contains[struct {
		Position Ref[Position3D]
	}]
}

type getSetSystemState5 struct {
	Entities Contains[struct {
		Position  Ref[Position3D]
		Velocity  Ref[Velocity3D]
		Health    Ref[Health2]
		Transform Ref[Transform]
		Inventory Ref[Inventory]
	}]
}

type getSetSystemState10 struct {
	Entities Contains[struct {
		Position    Ref[Position3D]
		Velocity    Ref[Velocity3D]
		Health      Ref[Health2]
		Transform   Ref[Transform]
		Inventory   Ref[Inventory]
		PlayerStats Ref[PlayerStats]
		AIBehavior  Ref[AIBehavior]
		Renderer    Ref[Renderer]
		Physics     Ref[Physics]
		NetworkSync Ref[NetworkSync]
	}]
}

// BenchmarkECS2_Iteration_Pure benchmarks pure iteration without get/set operations.
func BenchmarkECS2_Iteration_Pure(b *testing.B) {
	// Exact iteration - query for entities with exactly these components
	b.Run("Exact/1 component", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(Position3D{})
			ws := w.state

			// Create 100 entities
			for j := 0; j < 100; j++ {
				arch := Archetype1{}
				arch.CreateEntities(ws, Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
			}

			search := Exact[struct{ Position Ref[Position3D] }]{}
			_, _ = search.init(w)
			b.StartTimer()
			for _, result := range search.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})

	b.Run("Exact/5 components", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(Position3D{}, Velocity3D{}, Health2{}, Transform{}, Inventory{})
			ws := w.state

			// Create 100 entities
			for j := 0; j < 100; j++ {
				arch := Archetype5{}
				arch.CreateEntities(ws,
					Position3D{X: float64(j), Y: float64(j), Z: float64(j)},
					Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)},
					Health2{Current: j, Max: 100},
					Transform{Scale: 1.0, Rotation: float64(j)},
					Inventory{Items: []string{"item"}, Capacity: 10})
			}

			search := Exact[struct {
				Position  Ref[Position3D]
				Velocity  Ref[Velocity3D]
				Health    Ref[Health2]
				Transform Ref[Transform]
				Inventory Ref[Inventory]
			}]{}
			_, _ = search.init(w)
			b.StartTimer()
			for _, result := range search.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})

	b.Run("Exact/10 components", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(
				Position3D{}, Velocity3D{}, Health2{}, Transform{}, Inventory{},
				PlayerStats{}, AIBehavior{}, Renderer{}, Physics{}, NetworkSync{},
			)
			ws := w.state

			// Create 100 entities
			for j := 0; j < 100; j++ {
				arch := Archetype10{}
				arch.CreateEntities(ws,
					Position3D{X: float64(j), Y: float64(j), Z: float64(j)},
					Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)},
					Health2{Current: j, Max: 100},
					Transform{Scale: 1.0, Rotation: float64(j)},
					Inventory{Items: []string{"item"}, Capacity: 10},
					PlayerStats{Level: j, Experience: j * 10, Strength: 10, Agility: 8},
					AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0},
					Renderer{Model: "model", Texture: "texture", Visible: true, ZIndex: 1},
					Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false},
					NetworkSync{PlayerID: "player", LastUpdate: int64(j), SyncRate: 30.0, IsDirty: false, Interpolate: true})
			}

			search := Exact[struct {
				Position    Ref[Position3D]
				Velocity    Ref[Velocity3D]
				Health      Ref[Health2]
				Transform   Ref[Transform]
				Inventory   Ref[Inventory]
				PlayerStats Ref[PlayerStats]
				AIBehavior  Ref[AIBehavior]
				Renderer    Ref[Renderer]
				Physics     Ref[Physics]
				NetworkSync Ref[NetworkSync]
			}]{}
			_, _ = search.init(w)
			b.StartTimer()
			for _, result := range search.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})

	// Contains iteration - query for subset of components from entities with more components
	b.Run("Contains/1 from 1 component entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(Position3D{})
			ws := w.state

			// Create 100 entities
			for j := 0; j < 100; j++ {
				arch := Archetype1{}
				arch.CreateEntities(ws, Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
			}

			search := Contains[struct{ Position Ref[Position3D] }]{}
			_, _ = search.init(w)
			b.StartTimer()
			for _, result := range search.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})

	b.Run("Contains/3 from 5 component entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(Position3D{}, Velocity3D{}, Health2{}, Transform{}, Inventory{})
			ws := w.state

			// Create 100 entities
			for j := 0; j < 100; j++ {
				arch := Archetype5{}
				arch.CreateEntities(ws,
					Position3D{X: float64(j), Y: float64(j), Z: float64(j)},
					Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)},
					Health2{Current: j, Max: 100},
					Transform{Scale: 1.0, Rotation: float64(j)},
					Inventory{Items: []string{"item"}, Capacity: 10})
			}

			search := Contains[struct {
				Position Ref[Position3D]
				Velocity Ref[Velocity3D]
				Health   Ref[Health2]
			}]{}
			_, _ = search.init(w)
			b.StartTimer()
			for _, result := range search.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})

	b.Run("Contains/6 from 10 component entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(
				Position3D{}, Velocity3D{}, Health2{}, Transform{}, Inventory{},
				PlayerStats{}, AIBehavior{}, Renderer{}, Physics{}, NetworkSync{},
			)
			ws := w.state

			// Create 100 entities
			for j := 0; j < 100; j++ {
				arch := Archetype10{}
				arch.CreateEntities(ws,
					Position3D{X: float64(j), Y: float64(j), Z: float64(j)},
					Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)},
					Health2{Current: j, Max: 100},
					Transform{Scale: 1.0, Rotation: float64(j)},
					Inventory{Items: []string{"item"}, Capacity: 10},
					PlayerStats{Level: j, Experience: j * 10, Strength: 10, Agility: 8},
					AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0},
					Renderer{Model: "model", Texture: "texture", Visible: true, ZIndex: 1},
					Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false},
					NetworkSync{PlayerID: "player", LastUpdate: int64(j), SyncRate: 30.0, IsDirty: false, Interpolate: true})
			}

			search := Contains[struct {
				Position    Ref[Position3D]
				Velocity    Ref[Velocity3D]
				Health      Ref[Health2]
				Transform   Ref[Transform]
				Inventory   Ref[Inventory]
				PlayerStats Ref[PlayerStats]
			}]{}
			_, _ = search.init(w)
			b.StartTimer()
			for _, result := range search.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})
}

// BenchmarkECS2_Iteration_GetSet benchmarks system iteration with get/set operations.
func BenchmarkECS2_Iteration_GetSet(b *testing.B) {
	b.Run("1 component 100 entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(Position3D{})

			// Setup system: create 100 entities
			setupSystem := func(state *getSetSystemState1) error {
				for j := 0; j < 100; j++ {
					_, entity := state.Entities.Create()
					entity.Position.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
				}
				return nil
			}

			// GetSet system: get and set components
			getSetSystem := func(state *getSetSystemState1) error {
				b.StartTimer()
				for _, entity := range state.Entities.Iter() {
					// Get the position
					pos := entity.Position.Get()
					// Mutate it
					pos.X += 1.0
					// Set it back
					entity.Position.Set(pos)
				}
				b.StopTimer()
				return nil
			}

			RegisterSystem(w, setupSystem, WithHook(Init))
			RegisterSystem(w, getSetSystem)

			w.InitSchedulers()
			_ = w.InitSystems()

			_, _ = w.Tick(nil)
		}
	})

	b.Run("5 components 100 entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(Position3D{}, Velocity3D{}, Health2{}, Transform{}, Inventory{})

			// Setup system: create 100 entities
			setupSystem := func(state *getSetSystemState5) error {
				for j := 0; j < 100; j++ {
					_, entity := state.Entities.Create()
					entity.Position.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
					entity.Velocity.Set(Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)})
					entity.Health.Set(Health2{Current: j, Max: 100})
					entity.Transform.Set(Transform{Scale: 1.0, Rotation: float64(j)})
					entity.Inventory.Set(Inventory{Items: []string{"item"}, Capacity: 10})
				}
				return nil
			}

			// GetSet system: get position, set velocity
			getSetSystem := func(state *getSetSystemState5) error {
				b.StartTimer()
				for _, entity := range state.Entities.Iter() {
					// Get position
					pos := entity.Position.Get()
					// Get and mutate velocity based on position
					vel := entity.Velocity.Get()
					vel.X = pos.X * 0.1
					vel.Y = pos.Y * 0.1
					// Set velocity back
					entity.Velocity.Set(vel)
				}
				b.StopTimer()
				return nil
			}

			RegisterSystem(w, setupSystem, WithHook(Init))
			RegisterSystem(w, getSetSystem)

			w.InitSchedulers()
			_ = w.InitSystems()

			_, _ = w.Tick(nil)
		}
	})

	b.Run("10 components 100 entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := setup2(
				Position3D{}, Velocity3D{}, Health2{}, Transform{}, Inventory{},
				PlayerStats{}, AIBehavior{}, Renderer{}, Physics{}, NetworkSync{},
			)

			// Setup system: create 100 entities
			setupSystem := func(state *getSetSystemState10) error {
				for j := 0; j < 100; j++ {
					_, entity := state.Entities.Create()
					entity.Position.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
					entity.Velocity.Set(Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)})
					entity.Health.Set(Health2{Current: j, Max: 100})
					entity.Transform.Set(Transform{Scale: 1.0, Rotation: float64(j)})
					entity.Inventory.Set(Inventory{Items: []string{"item"}, Capacity: 10})
					entity.PlayerStats.Set(PlayerStats{Level: j, Experience: j * 10, Strength: 10, Agility: 8})
					entity.AIBehavior.Set(AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0})
					entity.Renderer.Set(Renderer{Model: "model", Texture: "texture", Visible: true, ZIndex: 1})
					entity.Physics.Set(Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false})
					entity.NetworkSync.Set(NetworkSync{
						PlayerID: "player", LastUpdate: int64(j), SyncRate: 30.0,
						IsDirty: false, Interpolate: true,
					})
				}
				return nil
			}

			// GetSet system: get position and health, set physics and renderer
			getSetSystem := func(state *getSetSystemState10) error {
				b.StartTimer()
				for _, entity := range state.Entities.Iter() {
					// Get position and health
					pos := entity.Position.Get()
					health := entity.Health.Get()

					// Mutate physics based on position
					physics := entity.Physics.Get()
					physics.Mass = pos.X * 0.01
					entity.Physics.Set(physics)

					// Mutate renderer based on health
					renderer := entity.Renderer.Get()
					renderer.Visible = health.Current > 50
					entity.Renderer.Set(renderer)
				}
				b.StopTimer()
				return nil
			}

			RegisterSystem(w, setupSystem, WithHook(Init))
			RegisterSystem(w, getSetSystem)

			w.InitSchedulers()
			_ = w.InitSystems()

			_, _ = w.Tick(nil)
		}
	})
}

func setup2(components ...Component) *World {
	w := NewWorld()
	ws := w.state
	for _, c := range components {
		switch c.(type) {
		case Position3D:
			_, _ = registerComponent[Position3D](ws)
		case Velocity3D:
			_, _ = registerComponent[Velocity3D](ws)
		case Health2:
			_, _ = registerComponent[Health2](ws)
		case Transform:
			_, _ = registerComponent[Transform](ws)
		case Inventory:
			_, _ = registerComponent[Inventory](ws)
		case PlayerStats:
			_, _ = registerComponent[PlayerStats](ws)
		case AIBehavior:
			_, _ = registerComponent[AIBehavior](ws)
		case Renderer:
			_, _ = registerComponent[Renderer](ws)
		case Physics:
			_, _ = registerComponent[Physics](ws)
		case NetworkSync:
			_, _ = registerComponent[NetworkSync](ws)
		}
	}
	return w
}

// Archetypes

type Archetype1 struct{}

func (a1 Archetype1) CreateEntities(ws *worldState, c1 Position3D) EntityID {
	var bm bitmap.Bitmap

	// Set components in the bitmap.
	c1ID, _ := ws.components.getID(c1.Name())
	bm.Set(c1ID)

	// Allocate entity in the right archetype.
	eid := ws.newEntityWithArchetype(bm)

	// Set components.
	_ = setComponent(ws, eid, c1)

	return eid
}

type Archetype5 struct{}

func (a5 Archetype5) CreateEntities(
	ws *worldState, c1 Position3D, c2 Velocity3D, c3 Health2, c4 Transform, c5 Inventory,
) EntityID {
	var bm bitmap.Bitmap

	// Set components in the bitmap.
	c1ID, _ := ws.components.getID(c1.Name())
	bm.Set(c1ID)
	c2ID, _ := ws.components.getID(c2.Name())
	bm.Set(c2ID)
	c3ID, _ := ws.components.getID(c3.Name())
	bm.Set(c3ID)
	c4ID, _ := ws.components.getID(c4.Name())
	bm.Set(c4ID)
	c5ID, _ := ws.components.getID(c5.Name())
	bm.Set(c5ID)

	// Allocate entity in the right archetype.
	eid := ws.newEntityWithArchetype(bm)

	// Set components.
	_ = setComponent(ws, eid, c1)
	_ = setComponent(ws, eid, c2)
	_ = setComponent(ws, eid, c3)
	_ = setComponent(ws, eid, c4)
	_ = setComponent(ws, eid, c5)

	return eid
}

type Archetype10 struct{}

func (a10 Archetype10) CreateEntities(
	ws *worldState, c1 Position3D, c2 Velocity3D, c3 Health2, c4 Transform, c5 Inventory,
	c6 PlayerStats, c7 AIBehavior, c8 Renderer, c9 Physics, c10 NetworkSync,
) EntityID {
	var bm bitmap.Bitmap

	// Set components in the bitmap.
	c1ID, _ := ws.components.getID(c1.Name())
	bm.Set(c1ID)
	c2ID, _ := ws.components.getID(c2.Name())
	bm.Set(c2ID)
	c3ID, _ := ws.components.getID(c3.Name())
	bm.Set(c3ID)
	c4ID, _ := ws.components.getID(c4.Name())
	bm.Set(c4ID)
	c5ID, _ := ws.components.getID(c5.Name())
	bm.Set(c5ID)
	c6ID, _ := ws.components.getID(c6.Name())
	bm.Set(c6ID)
	c7ID, _ := ws.components.getID(c7.Name())
	bm.Set(c7ID)
	c8ID, _ := ws.components.getID(c8.Name())
	bm.Set(c8ID)
	c9ID, _ := ws.components.getID(c9.Name())
	bm.Set(c9ID)
	c10ID, _ := ws.components.getID(c10.Name())
	bm.Set(c10ID)

	// Allocate entity in the right archetype.
	eid := ws.newEntityWithArchetype(bm)

	// Set components.
	_ = setComponent(ws, eid, c1)
	_ = setComponent(ws, eid, c2)
	_ = setComponent(ws, eid, c3)
	_ = setComponent(ws, eid, c4)
	_ = setComponent(ws, eid, c5)
	_ = setComponent(ws, eid, c6)
	_ = setComponent(ws, eid, c7)
	_ = setComponent(ws, eid, c8)
	_ = setComponent(ws, eid, c9)
	_ = setComponent(ws, eid, c10)

	return eid
}

type Archetype2 struct{}

func (a2 Archetype2) CreateEntities(ws *worldState, c1 Position3D, c2 Velocity3D) EntityID {
	var bm bitmap.Bitmap

	// Set components in the bitmap.
	c1ID, _ := ws.components.getID(c1.Name())
	bm.Set(c1ID)
	c2ID, _ := ws.components.getID(c2.Name())
	bm.Set(c2ID)

	// Allocate entity in the right archetype.
	eid := ws.newEntityWithArchetype(bm)

	// Set components.
	_ = setComponent(ws, eid, c1)
	_ = setComponent(ws, eid, c2)

	return eid
}

// Test components for benchmarking - identical to original bench_internal_test.go

// Position3D represents 3D spatial coordinates.
type Position3D struct {
	X, Y, Z float64
}

func (Position3D) Name() string { return "Position" }

// Velocity3D represents 3D movement speed.
type Velocity3D struct {
	X, Y, Z float64
}

func (Velocity3D) Name() string { return "Velocity" }

// Health2 represents entity health state.
type Health2 struct {
	Current, Max int
}

func (Health2) Name() string { return "Health" }

// Transform represents scale and rotation.
type Transform struct {
	Scale    float64
	Rotation float64
}

func (Transform) Name() string { return "Transform" }

// Inventory represents item storage.
type Inventory struct {
	Items    []string
	Capacity int
}

func (Inventory) Name() string { return "Inventory" }

// PlayerStats represents character progression.
type PlayerStats struct {
	Level      int
	Experience int
	Strength   int
	Agility    int
}

func (PlayerStats) Name() string { return "PlayerStats" }

// AIBehavior represents AI state and targeting.
type AIBehavior struct {
	State       string
	Target      EntityID
	Aggression  float64
	PatrolRange float64
}

func (AIBehavior) Name() string { return "AIBehavior" }

// Renderer represents visual rendering data.
type Renderer struct {
	Model   string
	Texture string
	Visible bool
	ZIndex  int
}

func (Renderer) Name() string { return "Renderer" }

// Physics represents physical properties.
type Physics struct {
	Mass        float64
	Friction    float64
	Restitution float64
	IsStatic    bool
}

func (Physics) Name() string { return "Physics" }

// NetworkSync represents network synchronization data.
type NetworkSync struct {
	PlayerID    string
	LastUpdate  int64
	SyncRate    float64
	IsDirty     bool
	Interpolate bool
}

func (NetworkSync) Name() string { return "NetworkSync" }
