package ecs

import (
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestArchetype creates an archetype with Health and Position components.
func createTestArchetype() *archetype {
	// Create a component manager and register test components
	cm := newComponentManager()
	_ = cm.register("Health", newColumnConstructor[Health]())
	_ = cm.register("Position", newColumnConstructor[Position]())

	// Create bitmap with both components
	var components bitmap.Bitmap
	components.Set(0) // Health
	components.Set(1) // Position

	// Create archetype with the test components
	arch := cm.createArchetype(0, components)
	return &arch
}

func createTestArchetypeComponents() []Component {
	return []Component{Health{Value: 100}, Position{X: 1, Y: 2}}
}

func createTestArchetypeWithExtraComponent() *archetype {
	cm := newComponentManager()
	_ = cm.register("Health", newColumnConstructor[Health]())
	_ = cm.register("Position", newColumnConstructor[Position]())
	_ = cm.register("Velocity", newColumnConstructor[Velocity]())

	var components bitmap.Bitmap
	components.Set(0) // Health
	components.Set(1) // Position
	components.Set(2) // Velocity

	arch := cm.createArchetype(0, components)
	return &arch
}

// TestEntityManager_NewEntity tests entity creation with various scenarios.
func TestEntityManager_NewEntity(t *testing.T) {
	t.Parallel() // Enable parallel testing

	tests := []struct {
		name       string
		arch       *archetype
		components []Component
		wantErr    bool
		wantPanic  bool
	}{
		{
			name:       "create first entity",
			arch:       createTestArchetype(),
			components: createTestArchetypeComponents(),
		},
		{
			name:       "create entity after recycling",
			arch:       createTestArchetype(),
			components: createTestArchetypeComponents(),
		},
		{
			name:       "create with unregistered component",
			arch:       createTestArchetype(),
			components: []Component{Health{}, PlayerTag{}},
			wantErr:    true,
		},
		{
			name:       "nil archetype should never be provided",
			arch:       nil,
			components: createTestArchetypeComponents(),
			wantPanic:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // Enable parallel testing for subtests
			em := newEntityManager()

			if tt.name == "create entity after recycling" {
				id, err := em.new(tt.arch, createTestArchetypeComponents())
				require.NoError(t, err)

				err = em.remove(id)
				require.NoError(t, err)
			}

			if tt.wantPanic {
				assert.Panics(t, func() {
					_, _ = em.new(tt.arch, tt.components)
				})
				return
			}

			id, err := em.new(tt.arch, tt.components)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, em.isAlive(id))
			assert.Equal(t, tt.arch, em.entityArch[id])
		})
	}
}

// TestEntityManager_RemoveEntity tests entity removal with various scenarios.
func TestEntityManager_RemoveEntity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(em *entityManager) EntityID
		wantErr bool
		err     error
	}{
		{
			name: "remove existing entity",
			setup: func(em *entityManager) EntityID {
				id, err := em.new(createTestArchetype(), createTestArchetypeComponents())
				require.NoError(t, err)
				return id
			},
		},
		{
			name: "remove non-existent entity",
			setup: func(_ *entityManager) EntityID {
				return 999
			},
			wantErr: true,
			err:     ErrEntityNotFound,
		},
		{
			name: "remove already removed entity",
			setup: func(em *entityManager) EntityID {
				id, err := em.new(createTestArchetype(), createTestArchetypeComponents())
				require.NoError(t, err)
				err = em.remove(id)
				require.NoError(t, err)
				return id
			},
			wantErr: true,
			err:     ErrEntityNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			em := newEntityManager()
			id := tt.setup(&em)

			err := em.remove(id)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.err)
				return
			}

			require.NoError(t, err)
			assert.False(t, em.isAlive(id))
			_, exists := em.entityArch[id]
			assert.False(t, exists)
		})
	}
}

// TestEntityManager_MoveEntity tests moving entities between archetypes.
func TestEntityManager_MoveEntity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func() (*entityManager, EntityID, *archetype, []Component)
		wantErr bool
		err     error
	}{
		{
			name: "move entity to new archetype",
			setup: func() (*entityManager, EntityID, *archetype, []Component) {
				em := newEntityManager()

				oldComps := createTestArchetypeComponents()
				oldArch := createTestArchetype()

				id, err := em.new(oldArch, oldComps)
				require.NoError(t, err)

				newArch := createTestArchetypeWithExtraComponent()
				newComps := []Component{Health{Value: 100}, Position{X: 1, Y: 2}, Velocity{X: 1, Y: 2}}
				return &em, id, newArch, newComps
			},
		},
		{
			name: "move non-existent entity",
			setup: func() (*entityManager, EntityID, *archetype, []Component) {
				em := newEntityManager()
				newArch := createTestArchetypeWithExtraComponent()
				return &em, 999, newArch, []Component{Velocity{X: 1, Y: 2}}
			},
			wantErr: true,
			err:     ErrEntityNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			em, entity, newArch, newComps := tt.setup()

			err := em.move(entity, newArch, newComps)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, newArch, em.entityArch[entity])
		})
	}
}

// TestEntityManager_GetArchetype tests retrieving archetypes for entities.
func TestEntityManager_GetArchetype(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(*entityManager) EntityID
		wantErr bool
		err     error
	}{
		{
			name: "get archetype of existing entity",
			setup: func(em *entityManager) EntityID {
				id, _ := em.new(createTestArchetype(), createTestArchetypeComponents())
				return id
			},
		},
		{
			name: "get archetype of non-existent entity",
			setup: func(_ *entityManager) EntityID {
				return 999
			},
			wantErr: true,
			err:     ErrEntityNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			em := newEntityManager()
			id := tt.setup(&em)

			arch, err := em.getArchetype(id)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, em.entityArch[id], arch)
		})
	}
}
