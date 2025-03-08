package cardinal

import (
	"encoding/json"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/types"
)

// Test component types.
type TestComponentA struct {
	Value string
}

type TestComponentB struct {
	Counter int
}

func (TestComponentA) Name() string { return "test_component_a" }
func (TestComponentB) Name() string { return "test_component_b" }

type TestTask struct {
	Value string
}

func (TestTask) Name() string { return "test_task" }

func (t TestTask) Handle(_ WorldContext) error {
	return nil
}

// TestGetAllEntities verifies that the GetAllEntities function returns all entities
// and their components as expected.
func TestGetAllEntities(t *testing.T) {
	// Setup a test fixture
	tf := NewTestFixture(t, nil)
	world := tf.World

	// Register test components
	assert.NilError(t, RegisterComponent[TestComponentA](world))
	assert.NilError(t, RegisterComponent[TestComponentB](world))

	// Register a task
	assert.NilError(t, RegisterTask[TestTask](world))

	// Start the world
	tf.StartWorld()

	// Create world context
	wCtx := NewWorldContext(world)

	// Create test entities
	entity1, err := Create(wCtx, TestComponentA{Value: "Entity 1"}, TestComponentB{Counter: 10})
	assert.NilError(t, err)

	entity2, err := Create(wCtx, TestComponentA{Value: "Entity 2"}, TestComponentB{Counter: 20})
	assert.NilError(t, err)

	// Create a persona
	tf.CreatePersona("testpersona", "testaddress")

	// Create a task
	err = wCtx.ScheduleTickTask(1000, TestTask{Value: "Task 1"})
	assert.NilError(t, err)

	// Verify that all 4 entities are created
	count, err := NewSearch().Entity(filter.All()).Count(wCtx)
	assert.NilError(t, err)
	assert.Equal(t, 4, count, "Expected 4 entities, got %d", count)

	// Call GetAllEntities
	entities, err := wCtx.GetAllEntities()
	assert.NilError(t, err)

	// Verify results
	assert.Equal(t, 2, len(entities), "Expected 2 entities, got %d", len(entities))

	// Check that we have the expected entities
	_, hasEntity1 := entities[entity1]
	_, hasEntity2 := entities[entity2]

	// Check if there's a persona entity with a signer component
	hasSignerEntity := false
	for _, components := range entities {
		// Check if this entity has a signer component
		_, hasSigner := components[component.SignerComponent{}.Name()]
		if hasSigner {
			hasSignerEntity = true
			break
		}
	}

	// Check if there's a task entity with a task component
	hasTaskEntity := false
	for _, components := range entities {
		// Check if this entity has a task component
		_, hasTask := components[taskMetadata{}.Name()]
		if hasTask {
			hasTaskEntity = true
			break
		}
	}

	assert.Equal(t, true, hasEntity1, "Entity 1 should be included in results")
	assert.Equal(t, true, hasEntity2, "Entity 2 should be included in results")
	assert.Equal(t, false, hasSignerEntity, "Signer entity should be excluded from results")
	assert.Equal(t, false, hasTaskEntity, "Task entity should be excluded from results")

	// Verify that the components have the expected values
	// Note: We need to convert the raw JSON back to our component types
	if hasEntity1 {
		entity1Components := entities[entity1]

		// Verify TestComponentA
		var compA TestComponentA
		compAJSON, ok := entity1Components["test_component_a"].(json.RawMessage)
		assert.Equal(t, true, ok, "TestComponentA should be present for entity1")
		assert.NilError(t, json.Unmarshal(compAJSON, &compA))
		assert.Equal(t, "Entity 1", compA.Value)

		// Verify TestComponentB
		var compB TestComponentB
		compBJSON, ok := entity1Components["test_component_b"].(json.RawMessage)
		assert.Equal(t, true, ok, "TestComponentB should be present for entity1")
		assert.NilError(t, json.Unmarshal(compBJSON, &compB))
		assert.Equal(t, 10, compB.Counter)
	}

	// Test that we can find entity1 using Search with the correct filter
	searchResult := NewSearch().Entity(
		filter.Contains(filter.Component[TestComponentA]()),
	)

	foundEntities := make([]bool, 2)
	err = searchResult.Each(wCtx, func(id types.EntityID) bool {
		if id == entity1 {
			foundEntities[0] = true
		} else if id == entity2 {
			foundEntities[1] = true
		}
		return true
	})
	assert.NilError(t, err)

	assert.Equal(t, true, foundEntities[0], "Entity 1 should be found via Search")
	assert.Equal(t, true, foundEntities[1], "Entity 2 should be found via Search")
}
