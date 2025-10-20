package ecs

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/cardinal/v1"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// archetypeID is the unique identifier for an archetype.
// It is used internally to track and manage archetypes efficiently.
type archetypeID = uint64

// archetype represents a collection of entities with the same component types.
// NOTE: We store the componentTypeCount instead of using Bitmap.Count() because counting bits
// is O(n). This saves us around 10ns/op, which is 10x speed up in low # of components.
// We store columns in a slice instead of a map because it's faster for small # of components.
type archetype struct {
	id                 archetypeID   // The archetype ID
	entities           bitmap.Bitmap // Bitmap representing entities
	components         bitmap.Bitmap // Bitmap representing component types
	columns            []any         // Columns for each component type
	componentTypeCount int           // Number of component types in the archetype
}

// newArchetype creates an archetype for the given component types. Passed in columns length must
// be equal to the number of component types. We intentionally return the archetype instead of a
// pointer to it because we want to avoid having to do pointer dereference since the user of
// newArchetype wants to store the values directly in a slice.
func newArchetype(id archetypeID, components bitmap.Bitmap, columns []any) archetype {
	assert.That(len(columns) == components.Count(), "column length doesn't match component count")
	return archetype{
		id:                 id,
		components:         components,
		columns:            columns,
		componentTypeCount: components.Count(),
	}
}

// newEntity adds the entity to the archetype. It expects the user to be a responsible adult
// (as this is an internal function) and that the components provided corresponds to the component types in the
// archetype. We avoid checking the component types because we want to avoid the overhead of checking.
func (a *archetype) newEntity(entity EntityID, components []Component) error {
	if len(components) != a.componentTypeCount {
		return eris.Errorf("component count mismatch: expected %d, got %d", a.componentTypeCount, len(components))
	}

	a.entities.Set(uint32(entity))
	for _, component := range components {
		col, err := a.findAbstractColumn(component.Name())
		if err != nil {
			return eris.Wrapf(err, "failed to find column for component type %s", component.Name())
		}

		err = col.setAbstract(entity, component)
		if err != nil {
			return eris.Wrapf(err, "failed to set component for entity %d in column", entity)
		}
	}
	return nil
}

func (a *archetype) updateEntity(entity EntityID, components []Component) error {
	if !a.hasEntity(entity) {
		return eris.Errorf("entity %d not in archetype", entity)
	}

	if len(components) != a.componentTypeCount {
		return eris.Errorf("component count mismatch: expected %d, got %d", a.componentTypeCount, len(components))
	}

	for _, component := range components {
		col, err := a.findAbstractColumn(component.Name())
		if err != nil {
			return eris.Wrapf(err, "failed to find column for component type %s", component.Name())
		}
		err = col.setAbstract(entity, component)
		if err != nil {
			return eris.Wrapf(err, "failed to set component for entity %d in column", entity)
		}
	}
	return nil
}

// removeEntity removes an entity from the archetype.
// If the entity is not in the archetype, it is no-op.
func (a *archetype) removeEntity(entity EntityID) {
	if !a.hasEntity(entity) {
		return
	}

	// Remove all entity components.
	for _, col := range a.columns {
		toAbstractColumn(col).remove(entity)
	}

	// Remove entity from archetype's entity map.
	a.entities.Remove(uint32(entity))
}

// collectComponents returns all components for the given entity.
// If exclude is provided, the components in exclude are not returned.
func (a *archetype) collectComponents(entity EntityID, exclude ...string) []Component {
	components := make([]Component, 0, a.componentTypeCount)
	for _, col := range a.columns {
		c := toAbstractColumn(col)
		if slices.Contains(exclude, c.componentName()) {
			continue
		}
		component, ok := c.getAbstract(entity)
		if !ok {
			continue
		}
		components = append(components, component)
	}
	return components
}

// matches checks if this archetype matches the given component types using bitmap operations.
func (a *archetype) matches(components bitmap.Bitmap) bool {
	if components.Count() != a.componentTypeCount {
		return false
	}
	return a.hasComponents(components)
}

// hasComponent checks if the archetype contains a specific component type.
func (a *archetype) hasComponent(componentTypeID componentID) bool {
	return a.components.Contains(componentTypeID)
}

// hasComponents checks if the archetype contains all the given component types.
func (a *archetype) hasComponents(comps bitmap.Bitmap) bool {
	var intersect bitmap.Bitmap
	comps.Clone(&intersect)
	intersect.And(a.components)
	return intersect.Count() == comps.Count()
}

// componentBitmap returns a bitmap of the component types in this archetype.
func (a *archetype) componentBitmap() bitmap.Bitmap {
	var b bitmap.Bitmap
	a.components.Clone(&b)
	return b
}

// hasEntity checks if the archetype contains the given entity.
func (a *archetype) hasEntity(entity EntityID) bool {
	return a.entities.Contains(uint32(entity))
}

// findAbstractColumn finds the abstractColumn for the given component name.
func (a *archetype) findAbstractColumn(compName string) (abstractColumn, error) {
	for _, col := range a.columns {
		c := toAbstractColumn(col)
		if c.componentName() == compName {
			return c, nil
		}
	}
	return nil, eris.Errorf("column for component type %s not found in archetype", compName)
}

// getColumnFromArch returns the column for the given component type.
func getColumnFromArch[T Component](a *archetype) (*column[T], error) {
	var zero T
	for _, col := range a.columns {
		c := toAbstractColumn(col)
		if c.componentName() == zero.Name() {
			return c.(*column[T]), nil //nolint:errcheck // We know the type
		}
	}
	return nil, eris.Errorf("column for component type %s not found in archetype", zero.Name())
}

// -------------------------------------------------------------------------------------------------
// Serialization methods
// -------------------------------------------------------------------------------------------------

// serialize converts the archetype to a protobuf message for serialization.
func (a *archetype) serialize() (*cardinalv1.Archetype, error) {
	entitiesBitmap := a.entities.ToBytes()
	componentsBitmap := a.components.ToBytes()

	columns := make([]*cardinalv1.Column, len(a.columns))
	for i, col := range a.columns {
		c := toAbstractColumn(col)
		serializedCol, err := c.serialize()
		if err != nil {
			return nil, eris.Wrapf(err, "failed to serialize column %d", i)
		}
		columns[i] = serializedCol
	}

	return &cardinalv1.Archetype{
		Id:               a.id,
		EntitiesBitmap:   entitiesBitmap,
		ComponentsBitmap: componentsBitmap,
		Columns:          columns,
	}, nil
}

// deserialize populates the archetype from a protobuf message.
func (a *archetype) deserialize(pb *cardinalv1.Archetype, cm *componentManager) error {
	a.id = pb.GetId()
	a.entities = bitmap.FromBytes(pb.GetEntitiesBitmap())
	a.components = bitmap.FromBytes(pb.GetComponentsBitmap())
	a.componentTypeCount = a.components.Count()

	a.columns = make([]any, len(pb.GetColumns()))
	for i, pbCol := range pb.GetColumns() {
		// Get column factory using component name.
		factory, err := cm.getColumnFactory(pbCol.GetComponentName())
		if err != nil {
			return eris.Wrapf(err, "failed to get column factory for component %s", pbCol.GetComponentName())
		}

		col := factory()
		c := toAbstractColumn(col)
		if err := c.deserialize(pbCol); err != nil {
			return eris.Wrapf(err, "failed to deserialize column %d", i)
		}
		a.columns[i] = col
	}
	return nil
}
