package ecs

import (
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWorldWithComponents(components ...any) *World {
	w := NewWorld()
	for _, c := range components {
		switch c.(type) {
		case Position:
			RegisterComponent[Position](w)
		case Velocity:
			RegisterComponent[Velocity](w)
		case Health:
			RegisterComponent[Health](w)
		}
	}
	return w
}

func TestECS_Create(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		components []Component
		wantPanic  bool
	}{
		{
			name: "create entity with single component",
			components: []Component{
				Position{X: 1, Y: 2},
			},
		},
		{
			name: "create entity with multiple components",
			components: []Component{
				Position{X: 1, Y: 2},
				Velocity{X: 3, Y: 4},
			},
		},
		{
			name:       "panic on no components",
			components: []Component{},
			wantPanic:  true,
		},
		{
			name: "panic on nil component",
			components: []Component{
				Position{X: 1, Y: 2},
				nil,
			},
			wantPanic: true,
		},
		{
			name: "panic on unregistered component type",
			components: []Component{
				Health{Value: 100},
			},
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := NewWorld()
			RegisterComponent[Position](w)
			RegisterComponent[Velocity](w)

			if tt.wantPanic {
				assert.Panics(t, func() {
					w.CustomTick(func(ws *WorldState) {
						Create(ws, tt.components...)
					})
				})
				return
			}

			searchPosition := Contains[struct{ Ref[Position] }]{}
			_, err := searchPosition.init(w)
			require.NoError(t, err)

			searchVelocity := Contains[struct{ Ref[Velocity] }]{}
			_, err = searchVelocity.init(w)
			require.NoError(t, err)

			var entity EntityID
			w.CustomTick(func(ws *WorldState) {
				entity = Create(ws, tt.components...)
				assert.True(t, Alive(ws, entity))

				// Verify all components were set correctly
				for _, c := range tt.components {
					switch comp := c.(type) {
					case Position:
						for _, pos := range searchPosition.Iter() {
							assert.Equal(t, comp, pos.Get())
						}
					case Velocity:
						for _, vel := range searchVelocity.Iter() {
							assert.Equal(t, comp, vel.Get())
						}
					}
				}
			})
		})
	}
}

type setComponentTest[T Component] struct {
	name          string
	setupEntity   func(*WorldState) EntityID
	setComponent  T
	expectPanic   bool
	validateState func(*testing.T, *World, *WorldState)
}

func TestECS_SetComponent(t *testing.T) {
	t.Parallel()

	tests1 := []setComponentTest[Velocity]{
		{
			name: "set new component type",
			setupEntity: func(ws *WorldState) EntityID {
				var entity EntityID
				Create(ws, Position{X: 1, Y: 2})
				return entity
			},
			setComponent: Velocity{X: 3, Y: 4},
			validateState: func(t *testing.T, w *World, ws *WorldState) {
				searchPosition := Contains[struct{ Ref[Position] }]{}
				_, err := searchPosition.init(w)
				require.NoError(t, err)

				for _, pos := range searchPosition.Iter() {
					assert.Equal(t, Position{X: 1, Y: 2}, pos.Get())
					break
				}

				searchVelocity := Contains[struct{ Ref[Velocity] }]{}
				_, err = searchVelocity.init(w)
				require.NoError(t, err)

				for _, vel := range searchVelocity.Iter() {
					assert.Equal(t, Velocity{X: 3, Y: 4}, vel.Get())
					break
				}
			},
		},
	}

	tests2 := []setComponentTest[Position]{
		{
			name: "update existing component",
			setupEntity: func(ws *WorldState) EntityID {
				var entity EntityID
				Create(ws, Position{X: 1, Y: 2})
				return entity
			},
			setComponent: Position{X: 5, Y: 6},
			validateState: func(t *testing.T, w *World, ws *WorldState) {
				search := Contains[struct{ Ref[Position] }]{}
				_, err := search.init(w)
				require.NoError(t, err)

				for _, pos := range search.Iter() {
					assert.Equal(t, Position{X: 5, Y: 6}, pos.Get())
					break
				}
			},
		},
		{
			name: "panic on set component on non-existent entity",
			setupEntity: func(ws *WorldState) EntityID {
				return EntityID(999)
			},
			setComponent: Position{X: 1, Y: 4},
			expectPanic:  true,
		},
		{
			name: "panic on set component after entity deleted",
			setupEntity: func(ws *WorldState) EntityID {
				var entity EntityID
				Create(ws, Position{X: 1, Y: 2})
				Destroy(ws, entity)
				return entity
			},
			setComponent: Position{X: 1, Y: 4},
			expectPanic:  true,
		},
	}

	tests3 := []setComponentTest[Health]{
		{
			name: "panic on set component on unregistered component type",
			setupEntity: func(ws *WorldState) EntityID {
				var entity EntityID
				Create(ws, Position{X: 1, Y: 2})
				return entity
			},
			setComponent: Health{Value: 100},
			expectPanic:  true,
		},
	}

	for _, tt := range tests1 {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := NewWorld()
			RegisterComponent[Position](w)
			RegisterComponent[Velocity](w)

			if tt.expectPanic {
				assert.Panics(t, func() {
					w.CustomTick(func(ws *WorldState) {
						entity := tt.setupEntity(ws)
						Set(ws, entity, tt.setComponent)
					})
				})
				return
			}

			w.CustomTick(func(ws *WorldState) {
				entity := tt.setupEntity(ws)

				Set(ws, entity, tt.setComponent)

				if tt.validateState != nil {
					tt.validateState(t, w, ws)
				}
			})
		})
	}

	for _, tt := range tests2 {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := NewWorld()
			RegisterComponent[Position](w)
			RegisterComponent[Velocity](w)

			if tt.expectPanic {
				assert.Panics(t, func() {
					w.CustomTick(func(ws *WorldState) {
						entity := tt.setupEntity(ws)
						Set(ws, entity, tt.setComponent)
					})
				})
				return
			}

			w.CustomTick(func(ws *WorldState) {
				entity := tt.setupEntity(ws)
				Set(ws, entity, tt.setComponent)
				if tt.validateState != nil {
					tt.validateState(t, w, ws)
				}
			})
		})
	}

	for _, tt := range tests3 {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := NewWorld()
			RegisterComponent[Position](w)
			RegisterComponent[Velocity](w)

			if tt.expectPanic {
				assert.Panics(t, func() {
					w.CustomTick(func(ws *WorldState) {
						entity := tt.setupEntity(ws)
						Set(ws, entity, tt.setComponent)
					})
				})
				return
			}

			w.CustomTick(func(ws *WorldState) {
				entity := tt.setupEntity(ws)
				Set(ws, entity, tt.setComponent)
				if tt.validateState != nil {
					tt.validateState(t, w, ws)
				}
			})
		})
	}
}

func TestECS_GetComponent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupWorld func(*WorldState) EntityID
		getType    any
		wantErr    error
		wantPanic  bool
		wantValue  Component
	}{
		{
			name: "get existing component",
			setupWorld: func(ws *WorldState) EntityID {
				var entity EntityID
				Create(ws, Position{X: 1, Y: 2})
				return entity
			},
			getType:   Position{},
			wantErr:   nil,
			wantValue: Position{X: 1, Y: 2},
		},
		{
			name: "get non-existent component type",
			setupWorld: func(ws *WorldState) EntityID {
				var entity EntityID
				Create(ws, Position{X: 1, Y: 2})
				return entity
			},
			getType:   Velocity{},
			wantPanic: true,
		},
		{
			name: "get from non-existent entity",
			setupWorld: func(ws *WorldState) EntityID {
				return EntityID(999)
			},
			getType: Position{},
			wantErr: ErrEntityNotFound,
		},
		{
			name: "get from deleted entity",
			setupWorld: func(ws *WorldState) EntityID {
				entity := Create(ws, Position{X: 1, Y: 1})
				Destroy(ws, entity)
				return entity
			},
			getType: Position{},
			wantErr: ErrEntityNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := NewWorld()
			RegisterComponent[Position](w)
			RegisterComponent[Velocity](w)

			testGet := func(ws *WorldState, entity EntityID, getComp any) {
				if tt.wantPanic {
					assert.Panics(t, func() {
						switch getComp.(type) {
						case Position:
							_, _ = Get[Position](ws, entity)
						case Velocity:
							_, _ = Get[Velocity](ws, entity)
						}
					})
					return
				}

				var comp Component
				var err error

				switch getComp.(type) {
				case Position:
					comp, err = Get[Position](ws, entity)
				case Velocity:
					comp, err = Get[Velocity](ws, entity)
				}

				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
					return
				}
				require.NoError(t, err)
				assert.Equal(t, tt.wantValue, comp)
			}

			w.CustomTick(func(ws *WorldState) {
				entity := tt.setupWorld(ws)
				testGet(ws, entity, tt.getType)
			})
		})
	}
}

func TestECS_DeleteEntity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupWorld func(*WorldState) EntityID
	}{
		{
			name: "delete existing entity",
			setupWorld: func(ws *WorldState) EntityID {
				var entity EntityID
				Create(ws, Position{X: 1, Y: 2}, Velocity{X: 3, Y: 4})
				return entity
			},
		},
		{
			name: "delete non-existent entity",
			setupWorld: func(ws *WorldState) EntityID {
				return EntityID(999)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := NewWorld()
			RegisterComponent[Position](w)
			RegisterComponent[Velocity](w)

			var entity EntityID
			w.CustomTick(func(ws *WorldState) {
				entity = tt.setupWorld(ws)
				Destroy(ws, entity)
				assert.False(t, Alive(ws, entity))
			})
		})
	}
}

func TestECS_RemoveComponent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupEntity   func(*WorldState) EntityID
		removeType    any
		validateState func(*testing.T, *World, *WorldState, EntityID)
	}{
		{
			name: "remove existing component",
			setupEntity: func(w *WorldState) EntityID {
				return Create(w, Position{X: 1, Y: 2}, Velocity{X: 3, Y: 4})
			},
			removeType: Position{},
			validateState: func(t *testing.T, w *World, ws *WorldState, _ EntityID) {
				searchPosition := Contains[struct{ Ref[Position] }]{}
				_, err := searchPosition.init(w)
				require.NoError(t, err)

				for range searchPosition.Iter() {
					assert.Fail(t, "Position component should not exist")
				}

				searchVelocity := Contains[struct{ Ref[Velocity] }]{}
				_, err = searchVelocity.init(w)
				require.NoError(t, err)

				for _, vel := range searchVelocity.Iter() {
					assert.Equal(t, Velocity{X: 3, Y: 4}, vel.Get())
					break
				}
			},
		},
		{
			name: "remove non-existent component",
			setupEntity: func(w *WorldState) EntityID {
				return Create(w, Position{X: 1, Y: 2})
			},
			removeType: Velocity{},
			validateState: func(t *testing.T, w *World, ws *WorldState, _ EntityID) {
				search := Contains[struct{ Ref[Position] }]{}
				_, err := search.init(w)
				require.NoError(t, err)

				for _, pos := range search.Iter() {
					assert.Equal(t, Position{X: 1, Y: 2}, pos.Get())
					break
				}
			},
		},
		{
			name: "remove components until entity is deleted",
			setupEntity: func(w *WorldState) EntityID {
				return Create(w, Position{X: 1, Y: 2}, Velocity{X: 3, Y: 4})
			},
			removeType: Position{},
			validateState: func(t *testing.T, w *World, ws *WorldState, e EntityID) {
				// First component removed, entity should still exist
				assert.True(t, Alive(ws, e))

				searchPosition := Contains[struct{ Ref[Position] }]{}
				_, err := searchPosition.init(w)
				require.NoError(t, err)

				for range searchPosition.Iter() {
					assert.Fail(t, "Position component should not exist")
				}

				searchVelocity := Contains[struct{ Ref[Velocity] }]{}
				_, err = searchVelocity.init(w)
				require.NoError(t, err)

				for _, vel := range searchVelocity.Iter() {
					assert.Equal(t, Velocity{X: 3, Y: 4}, vel.Get())
					break
				}
			},
		},
		{
			name: "remove all components to delete entity",
			setupEntity: func(w *WorldState) EntityID {
				return Create(w, Position{X: 1, Y: 2}, Velocity{X: 3, Y: 4})
			},
			removeType: Position{},
			validateState: func(t *testing.T, w *World, ws *WorldState, e EntityID) {
				// Entity should still exist
				assert.True(t, Alive(ws, e))

				searchPosition := Contains[struct{ Ref[Position] }]{}
				_, err := searchPosition.init(w)
				require.NoError(t, err)

				for range searchPosition.Iter() {
					assert.Fail(t, "Position component should not exist")
				}

				searchVelocity := Contains[struct{ Ref[Velocity] }]{}
				_, err = searchVelocity.init(w)
				require.NoError(t, err)

				for _, vel := range searchVelocity.Iter() {
					assert.Equal(t, Velocity{X: 3, Y: 4}, vel.Get())
					break
				}

				Remove[Velocity](ws, e)
				assert.False(t, Alive(ws, e))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := NewWorld()
			RegisterComponent[Position](w)
			RegisterComponent[Velocity](w)

			w.CustomTick(func(ws *WorldState) {
				entity := tt.setupEntity(ws)

				switch tt.removeType.(type) {
				case Position:
					Remove[Position](ws, entity)
				case Velocity:
					Remove[Velocity](ws, entity)
				}

				if tt.validateState != nil {
					tt.validateState(t, w, ws, entity)
				}
			})
		})
	}
}

func TestECS_HasComponent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupWorld func(*WorldState) EntityID
		checkType  any
		want       bool
	}{
		{
			name: "has existing component",
			setupWorld: func(w *WorldState) EntityID {
				return Create(w, Position{X: 1, Y: 2})
			},
			checkType: Position{},
			want:      true,
		},
		{
			name: "does not have component",
			setupWorld: func(w *WorldState) EntityID {
				return Create(w, Position{X: 1, Y: 2})
			},
			checkType: Velocity{},
			want:      false,
		},
		{
			name: "check non-existent entity",
			setupWorld: func(_ *WorldState) EntityID {
				return EntityID(999)
			},
			checkType: Position{},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := NewWorld()
			RegisterComponent[Position](w)
			RegisterComponent[Velocity](w)

			w.CustomTick(func(ws *WorldState) {
				entity := tt.setupWorld(ws)

				switch tt.checkType.(type) {
				case Position:
					got := Has[Position](ws, entity)
					assert.Equal(t, tt.want, got)
				case Velocity:
					got := Has[Velocity](ws, entity)
					assert.Equal(t, tt.want, got)
				}
			})
		})
	}
}

func TestECS_EntityExists(t *testing.T) {
	t.Parallel()

	w := NewWorld()
	RegisterComponent[Position](w)

	w.CustomTick(func(ws *WorldState) {
		entity := Create(ws, Position{X: 1, Y: 2})
		assert.True(t, Alive(ws, entity))
		assert.False(t, Alive(ws, EntityID(999)))
	})
}

func TestECS_EntityLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test func(*testing.T, *World, *WorldState)
	}{
		{
			name: "create-delete-create cycle",
			test: func(t *testing.T, w *World, ws *WorldState) {
				var e1, e2 EntityID

				// Create entity
				e1 = Create(ws, Position{X: 1, Y: 2})
				assert.True(t, Alive(ws, e1))

				// Delete it
				Destroy(ws, e1)
				assert.False(t, Alive(ws, e1))

				// Create new entity
				e2 = Create(ws, Position{X: 3, Y: 4})
				assert.True(t, Alive(ws, e2))

				// Verify components
				search := Exact[struct{ Ref[Position] }]{}
				_, err := search.init(w)
				require.NoError(t, err)

				for _, pos := range search.Iter() {
					assert.Equal(t, Position{X: 3, Y: 4}, pos.Get())
					break
				}
			},
		},
		{
			name: "component add-remove cycle",
			test: func(t *testing.T, w *World, ws *WorldState) {
				e := Create(ws, Position{X: 1, Y: 2})

				// Add and remove components multiple times
				for i := range 10 {
					// Add Velocity
					Set[Velocity](ws, e, Velocity{X: i, Y: i})
					assert.True(t, Has[Velocity](ws, e))

					// Remove Velocity
					Remove[Velocity](ws, e)
					assert.False(t, Has[Velocity](ws, e))

					// Original component should remain unchanged
					search := Contains[struct{ Ref[Position] }]{}
					_, err := search.init(w)
					require.NoError(t, err)

					for _, pos := range search.Iter() {
						assert.Equal(t, Position{X: 1, Y: 2}, pos.Get())
						break
					}
				}
			},
		},
		{
			name: "multiple component updates",
			test: func(t *testing.T, w *World, ws *WorldState) {
				e := Create(ws, Position{X: 1, Y: 2}, Velocity{X: 3, Y: 4})

				// Update components alternately
				for i := range 10 {
					Set[Position](ws, e, Position{X: i, Y: i})
					Set[Velocity](ws, e, Velocity{X: i * 2, Y: i * 2})

					// Verify both components after each update
					searchPosition := Contains[struct{ Ref[Position] }]{}
					_, err := searchPosition.init(w)
					require.NoError(t, err)

					for _, pos := range searchPosition.Iter() {
						assert.Equal(t, Position{X: i, Y: i}, pos.Get())
						break
					}

					searchVelocity := Contains[struct{ Ref[Velocity] }]{}
					_, err = searchVelocity.init(w)
					require.NoError(t, err)

					for _, vel := range searchVelocity.Iter() {
						assert.Equal(t, Velocity{X: i * 2, Y: i * 2}, vel.Get())
						break
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := NewWorld()
			RegisterComponent[Position](w)
			RegisterComponent[Velocity](w)

			w.CustomTick(func(ws *WorldState) {
				tt.test(t, w, ws)
			})
		})
	}
}

func TestECS_RegisterSystem(t *testing.T) {
	t.Parallel()
}

func BenchmarkECS_CreateEntity(b *testing.B) {
	benchmarks := []struct {
		name       string
		components []any
		create     func(*WorldState) EntityID
	}{
		{
			name:       "1 component",
			components: []any{Position{}},
			create: func(w *WorldState) EntityID {
				return Create(w, Position{X: 1, Y: 2})
			},
		},
		{
			name:       "2 components",
			components: []any{Position{}, Velocity{}},
			create: func(w *WorldState) EntityID {
				return Create(w, Position{X: 1, Y: 2}, Velocity{X: 3, Y: 4})
			},
		},
		{
			name:       "3 components",
			components: []any{Position{}, Velocity{}, Health{}},
			create: func(w *WorldState) EntityID {
				return Create(w, Position{X: 1, Y: 2}, Velocity{X: 3, Y: 4}, Health{Value: 100})
			},
		},
	}

	for _, bm := range benchmarks {
		// Benchmark full cost including archetype creation
		b.Run(bm.name+" with archetype creation", func(b *testing.B) {
			w := setupWorldWithComponents(bm.components...)

			// We use b.N here instead of b.Loop for more control on the benchmarks.
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StartTimer()
				w.CustomTick(func(ws *WorldState) {
					_ = bm.create(ws)
				})
				b.StopTimer()

				// Reset world
				w = setupWorldWithComponents(bm.components...)
			}
		})

		// Benchmark just the entity creation cost
		b.Run(bm.name+" existing archetype", func(b *testing.B) {
			w := setupWorldWithComponents(bm.components...)
			w.CustomTick(func(ws *WorldState) {
				bm.create(ws) // Create one entity to ensure archetype exists
			})

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StartTimer()
				w.CustomTick(func(ws *WorldState) {
					_ = bm.create(ws)
				})
				b.StopTimer()

				// Reset world
				w = setupWorldWithComponents(bm.components...)
				w.CustomTick(func(ws *WorldState) {
					bm.create(ws) // Create one entity to ensure archetype exists
				})
			}
		})
	}
}
