package cardinal

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
)

type entityState1 struct {
	Entities Contains[struct{ Position Ref[Position3D] }]
}

type entityState2 struct {
	Entities Contains[struct {
		Position Ref[Position3D]
		Velocity Ref[Velocity3D]
	}]
}

type entityState5 struct {
	Entities Contains[struct {
		Position  Ref[Position3D]
		Velocity  Ref[Velocity3D]
		Health    Ref[Health2]
		Transform Ref[Transform]
		Inventory Ref[Inventory]
	}]
}

type entityState10 struct {
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

func BenchmarkCardinal_Entity_Create(b *testing.B) {
	b.Run("1 component with archetype creation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState1{}
			mustInitSystemFields(b, w, state)

			b.StartTimer()
			_, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			b.StopTimer()
		}
	})

	b.Run("1 component existing archetype", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState1{}
			mustInitSystemFields(b, w, state)

			_, warmup := state.Entities.Create()
			warmup.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			_, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
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
			_, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Velocity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Health.Set(Health2{Current: 100, Max: 100})
			entity.Transform.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Inventory.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			b.StopTimer()
		}
	})

	b.Run("5 components existing archetype", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState5{}
			mustInitSystemFields(b, w, state)

			_, warmup := state.Entities.Create()
			warmup.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			warmup.Velocity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			warmup.Health.Set(Health2{Current: 100, Max: 100})
			warmup.Transform.Set(Transform{Scale: 1.0, Rotation: 0.0})
			warmup.Inventory.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})

			b.StartTimer()
			_, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Velocity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Health.Set(Health2{Current: 100, Max: 100})
			entity.Transform.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Inventory.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
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
			_, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Velocity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Health.Set(Health2{Current: 100, Max: 100})
			entity.Transform.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Inventory.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			entity.PlayerStats.Set(PlayerStats{Level: 5, Experience: 1000, Strength: 10, Agility: 8})
			entity.AIBehavior.Set(AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0})
			entity.Renderer.Set(Renderer{Model: "player", Texture: "player.png", Visible: true, ZIndex: 1})
			entity.Physics.Set(Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false})
			entity.NetworkSync.Set(NetworkSync{PlayerID: "player1", LastUpdate: 0, SyncRate: 30.0, IsDirty: false, Interpolate: true})
			b.StopTimer()
		}
	})

	b.Run("10 components existing archetype", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState10{}
			mustInitSystemFields(b, w, state)

			_, warmup := state.Entities.Create()
			warmup.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			warmup.Velocity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			warmup.Health.Set(Health2{Current: 100, Max: 100})
			warmup.Transform.Set(Transform{Scale: 1.0, Rotation: 0.0})
			warmup.Inventory.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			warmup.PlayerStats.Set(PlayerStats{Level: 5, Experience: 1000, Strength: 10, Agility: 8})
			warmup.AIBehavior.Set(AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0})
			warmup.Renderer.Set(Renderer{Model: "player", Texture: "player.png", Visible: true, ZIndex: 1})
			warmup.Physics.Set(Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false})
			warmup.NetworkSync.Set(NetworkSync{PlayerID: "player1", LastUpdate: 0, SyncRate: 30.0, IsDirty: false, Interpolate: true})

			b.StartTimer()
			_, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Velocity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Health.Set(Health2{Current: 100, Max: 100})
			entity.Transform.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Inventory.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			entity.PlayerStats.Set(PlayerStats{Level: 5, Experience: 1000, Strength: 10, Agility: 8})
			entity.AIBehavior.Set(AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0})
			entity.Renderer.Set(Renderer{Model: "player", Texture: "player.png", Visible: true, ZIndex: 1})
			entity.Physics.Set(Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false})
			entity.NetworkSync.Set(NetworkSync{PlayerID: "player1", LastUpdate: 0, SyncRate: 30.0, IsDirty: false, Interpolate: true})
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

			eid, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			_ = state.Entities.Destroy(eid)
			b.StopTimer()
		}
	})

	b.Run("5 components", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState5{}
			mustInitSystemFields(b, w, state)

			eid, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Velocity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Health.Set(Health2{Current: 100, Max: 100})
			entity.Transform.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Inventory.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})

			b.StartTimer()
			_ = state.Entities.Destroy(eid)
			b.StopTimer()
		}
	})

	b.Run("10 components", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState10{}
			mustInitSystemFields(b, w, state)

			eid, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Velocity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Health.Set(Health2{Current: 100, Max: 100})
			entity.Transform.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Inventory.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10})
			entity.PlayerStats.Set(PlayerStats{Level: 5, Experience: 1000, Strength: 10, Agility: 8})
			entity.AIBehavior.Set(AIBehavior{State: "idle", Target: 0, Aggression: 0.5, PatrolRange: 5.0})
			entity.Renderer.Set(Renderer{Model: "player", Texture: "player.png", Visible: true, ZIndex: 1})
			entity.Physics.Set(Physics{Mass: 1.0, Friction: 0.1, Restitution: 0.8, IsStatic: false})
			entity.NetworkSync.Set(NetworkSync{PlayerID: "player1", LastUpdate: 0, SyncRate: 30.0, IsDirty: false, Interpolate: true})

			b.StartTimer()
			_ = state.Entities.Destroy(eid)
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

			_, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Velocity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})

			b.StartTimer()
			entity.Position.Set(Position3D{X: 10.0, Y: 20.0, Z: 30.0})
			b.StopTimer()
		}
	})

	b.Run("add new component", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &struct {
				PositionOnly Contains[struct{ Position Ref[Position3D] }]
				PositionHP   Contains[struct {
					Position Ref[Position3D]
					Health   Ref[Health2]
				}]
			}{}
			mustInitSystemFields(b, w, state)

			eid, entity := state.PositionOnly.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			_ = ecs.Set(w.world, eid, Health2{Current: 100, Max: 100})
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

			_, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			entity.Position.Remove()
			b.StopTimer()
		}
	})

	b.Run("remove component from 5-component entity", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState5{}
			mustInitSystemFields(b, w, state)

			_, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Velocity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Health.Set(Health2{Current: 100, Max: 100})
			entity.Transform.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Inventory.Set(Inventory{Items: []string{"sword"}, Capacity: 10})

			b.StartTimer()
			entity.Velocity.Remove()
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

			_, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})

			b.StartTimer()
			_ = entity.Position.Get()
			b.StopTimer()
		}
	})

	b.Run("get component from 5-component entity", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()
			state := &entityState5{}
			mustInitSystemFields(b, w, state)

			_, entity := state.Entities.Create()
			entity.Position.Set(Position3D{X: 1.0, Y: 2.0, Z: 3.0})
			entity.Velocity.Set(Velocity3D{X: 0.5, Y: 1.0, Z: -0.2})
			entity.Health.Set(Health2{Current: 100, Max: 100})
			entity.Transform.Set(Transform{Scale: 1.0, Rotation: 0.0})
			entity.Inventory.Set(Inventory{Items: []string{"sword"}, Capacity: 10})

			b.StartTimer()
			_ = entity.Position.Get()
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
				Creator Contains[struct{ Position Ref[Position3D] }]
				Query   Exact[struct{ Position Ref[Position3D] }]
			}{}
			mustInitSystemFields(b, w, state)

			for j := 0; j < 100; j++ {
				_, entity := state.Creator.Create()
				entity.Position.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
			}

			b.StartTimer()
			for _, result := range state.Query.Iter() {
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
					Position  Ref[Position3D]
					Velocity  Ref[Velocity3D]
					Health    Ref[Health2]
					Transform Ref[Transform]
					Inventory Ref[Inventory]
				}]
				Query Exact[struct {
					Position  Ref[Position3D]
					Velocity  Ref[Velocity3D]
					Health    Ref[Health2]
					Transform Ref[Transform]
					Inventory Ref[Inventory]
				}]
			}{}
			mustInitSystemFields(b, w, state)

			for j := 0; j < 100; j++ {
				_, entity := state.Creator.Create()
				entity.Position.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
				entity.Velocity.Set(Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)})
				entity.Health.Set(Health2{Current: j, Max: 100})
				entity.Transform.Set(Transform{Scale: 1.0, Rotation: float64(j)})
				entity.Inventory.Set(Inventory{Items: []string{"item"}, Capacity: 10})
			}

			b.StartTimer()
			for _, result := range state.Query.Iter() {
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
				Query Exact[struct {
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
			}{}
			mustInitSystemFields(b, w, state)

			for j := 0; j < 100; j++ {
				_, entity := state.Creator.Create()
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

			b.StartTimer()
			for _, result := range state.Query.Iter() {
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
				Creator Contains[struct{ Position Ref[Position3D] }]
				Query   Contains[struct{ Position Ref[Position3D] }]
			}{}
			mustInitSystemFields(b, w, state)

			for j := 0; j < 100; j++ {
				_, entity := state.Creator.Create()
				entity.Position.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
			}

			b.StartTimer()
			for _, result := range state.Query.Iter() {
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
					Position  Ref[Position3D]
					Velocity  Ref[Velocity3D]
					Health    Ref[Health2]
					Transform Ref[Transform]
					Inventory Ref[Inventory]
				}]
				Query Contains[struct {
					Position Ref[Position3D]
					Velocity Ref[Velocity3D]
					Health   Ref[Health2]
				}]
			}{}
			mustInitSystemFields(b, w, state)

			for j := 0; j < 100; j++ {
				_, entity := state.Creator.Create()
				entity.Position.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
				entity.Velocity.Set(Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)})
				entity.Health.Set(Health2{Current: j, Max: 100})
				entity.Transform.Set(Transform{Scale: 1.0, Rotation: float64(j)})
				entity.Inventory.Set(Inventory{Items: []string{"item"}, Capacity: 10})
			}

			b.StartTimer()
			for _, result := range state.Query.Iter() {
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
				Query Contains[struct {
					Position    Ref[Position3D]
					Velocity    Ref[Velocity3D]
					Health      Ref[Health2]
					Transform   Ref[Transform]
					Inventory   Ref[Inventory]
					PlayerStats Ref[PlayerStats]
				}]
			}{}
			mustInitSystemFields(b, w, state)

			for j := 0; j < 100; j++ {
				_, entity := state.Creator.Create()
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

			b.StartTimer()
			for _, result := range state.Query.Iter() {
				_ = result
			}
			b.StopTimer()
		}
	})
}

type getSetSystemState1 struct {
	BaseSystemState
	Entities Contains[struct {
		Position Ref[Position3D]
	}]
}

type getSetSystemState5 struct {
	BaseSystemState
	Entities Contains[struct {
		Position  Ref[Position3D]
		Velocity  Ref[Velocity3D]
		Health    Ref[Health2]
		Transform Ref[Transform]
		Inventory Ref[Inventory]
	}]
}

type getSetSystemState10 struct {
	BaseSystemState
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

func BenchmarkCardinal_Iteration_GetSet(b *testing.B) {
	b.Run("1 component 100 entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()

			RegisterSystem(w, func(state *getSetSystemState1) {
				for j := 0; j < 100; j++ {
					_, entity := state.Entities.Create()
					entity.Position.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
				}
			}, WithHook(Init))

			RegisterSystem(w, func(state *getSetSystemState1) {
				b.StartTimer()
				for _, entity := range state.Entities.Iter() {
					pos := entity.Position.Get()
					pos.X += 1.0
					entity.Position.Set(pos)
				}
				b.StopTimer()
			}, WithHook(Update))

			w.world.Init()
			_ = w.world.Tick()
			_ = w.world.Tick()
		}
	})

	b.Run("5 components 100 entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()

			RegisterSystem(w, func(state *getSetSystemState5) {
				for j := 0; j < 100; j++ {
					_, entity := state.Entities.Create()
					entity.Position.Set(Position3D{X: float64(j), Y: float64(j), Z: float64(j)})
					entity.Velocity.Set(Velocity3D{X: float64(j), Y: float64(j), Z: float64(j)})
					entity.Health.Set(Health2{Current: j, Max: 100})
					entity.Transform.Set(Transform{Scale: 1.0, Rotation: float64(j)})
					entity.Inventory.Set(Inventory{Items: []string{"item"}, Capacity: 10})
				}
			}, WithHook(Init))

			RegisterSystem(w, func(state *getSetSystemState5) {
				b.StartTimer()
				for _, entity := range state.Entities.Iter() {
					pos := entity.Position.Get()
					vel := entity.Velocity.Get()
					vel.X = pos.X * 0.1
					vel.Y = pos.Y * 0.1
					entity.Velocity.Set(vel)
				}
				b.StopTimer()
			}, WithHook(Update))

			w.world.Init()
			_ = w.world.Tick()
			_ = w.world.Tick()
		}
	})

	b.Run("10 components 100 entities", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := newBenchWorld()

			RegisterSystem(w, func(state *getSetSystemState10) {
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
			}, WithHook(Init))

			RegisterSystem(w, func(state *getSetSystemState10) {
				b.StartTimer()
				for _, entity := range state.Entities.Iter() {
					pos := entity.Position.Get()
					health := entity.Health.Get()

					physics := entity.Physics.Get()
					physics.Mass = pos.X * 0.01
					entity.Physics.Set(physics)

					renderer := entity.Renderer.Get()
					renderer.Visible = health.Current > 50
					entity.Renderer.Set(renderer)
				}
				b.StopTimer()
			}, WithHook(Update))

			w.world.Init()
			_ = w.world.Tick()
			_ = w.world.Tick()
		}
	})
}

func newBenchWorld() *World {
	return &World{world: ecs.NewWorld()}
}

func mustInitSystemFields[T any](b testing.TB, world *World, state *T) {
	b.Helper()
	_, _, err := initSystemFields(state, world)
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
