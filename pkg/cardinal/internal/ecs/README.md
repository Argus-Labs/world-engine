# Go ECS (Entity Component System)

A high-performance Entity Component System (ECS) implementation in Go, designed for efficient game development and simulation systems.

## Architecture Overview

This ECS implementation follows an archetype-based design pattern, optimized for cache-friendly data layouts and efficient component access. The system is built around several key concepts:

```text
World
├── ComponentManager
│   ├── Component Registry (map[string]ComponentTypeID)
│   └── Column Factory ([]func() any)
│
├── Entity Table ([]entityRecord, one per index: archetype + generation)
│   └── Free Indices ([]uint32, min-heap)
│
└── Archetypes ([]archetype)
    └── archetype
       ├── Component Type Bitmap
       ├── Rows (sparseSet: entity index to row)
       ├── Entities ([]EntityID, dense, one per row)
       └── Columns ([]abstractColumn)
           ├── Column<T1> ([]T1, one per row)
           └── Column<T2> ([]T2, one per row)
```

### Core Components

1. **World (`World`)**:
   - The root container managing all ECS state
   - Handles component type registration
   - Manages entity lifecycle and component operations
   - Maintains archetype organization

2. **Entities (entity table)**:
   - `EntityID` is a `uint64`: the index in the low 32 bits and the generation in the high 32 bits
   - The entity table holds one entry per index: the entity's archetype and the index's current
     generation. A lookup checks both, so it is one read from one slice
   - Destroying an entity advances its index's generation. An ID kept past that point resolves as
     not found, like a destroyed entity, even after a new entity reuses the index
   - Generations start at 1, so the zero ID is never valid
   - At most 2^32 - 1 indices (0 to `maxEntityIndex`), the same limit as before generations
   - Freed indices are reused smallest first (a min-heap), so IDs depend only on the operations, not
     on the order entities were freed. Replays and restores produce the same IDs
   - A generation that passes its maximum wraps back to 1, skipping zero. An old ID can then match
     again only after its index has been reused 2^32 - 1 times
   - A snapshot stores every index's generation. Restore rebuilds the free indices from them, so IDs
     taken before a snapshot are still valid (or still stale) after restore
   - A world reset restarts generations, so a reset world issues the same IDs as a new one

3. **Components**:
   - Pure data containers attachable to entities
   - Implements the `Component` interface
   - Must provide a unique string identifier via `Name()`
   - Registered with the world before use
   - Stored in type-safe columns within archetypes

4. **Archetypes**:
   - Groups entities sharing the same component types
   - Uses bitmap-based component type tracking
   - Enables efficient component operations and queries
   - Automatically managed during entity mutations

5. **Columns**:
   - Type-safe component storage, one column per component type in the archetype
   - Hold only component data. Row i of every column belongs to the entity in row i of the
     archetype's entities

### Key Design Features

1. **Sparse Set Pattern**:
   - Each archetype is a sparse set: `rows` is the sparse array (entity index to row), and
     `entities` plus the columns are the dense arrays
   - Removing an entity moves the last row into its place, so the dense arrays stay packed and
     iteration touches only live entities
   - The entity table is not a sparse set. It only answers which archetype an entity is in, so it
     is one array indexed by entity index, with no dense half. Iteration already happens per
     archetype

2. **Type Safety**:
   - Compile-time type checking for components
   - Generic implementations for type-safe operations
   - Clear error handling for runtime safety
   - Strong typing for component registration

3. **Memory Efficiency**:
   - Component data stored contiguously
   - Entity ID recycling
   - Sparse storage for component data
   - Bitmap-based type tracking
   - Cache-friendly data layouts

4. **Performance Optimizations**:
   - O(1) component access
   - Efficient entity creation and deletion
   - Fast iteration over components
   - Cache-coherent data structures
   - Minimal runtime overhead

## Internal Typed Operations

Component access belongs to `worldState` through `setComponent`, `getComponent[T]`,
and `removeComponent[T]`. Component and system-event managers use `register[T](name)`
to create storage for the selected type. Callers no longer supply factories.

System-event managers expose `enqueue(event)` and `get[T](name)`. The model tests use
these same typed methods. The shared queue interface only supports clearing and
checking lengths across event types. Component column factories remain necessary
for creating archetypes and restoring snapshots from runtime component IDs.

## Usage Example

```go
// Define a component
type Position struct{ X, Y float32 }
func (Position) Name() string { return "Position" }

// Create a world and register components
world := ecs.NewWorld()
world.RegisterComponent[Position]()

// Create an entity and attach a component
entity := world.Create()
world.Set(entity, Position{X: 1, Y: 2})

// Modify components
world.Set(entity, Position{X: 3, Y: 4})

// Query components
pos, err := world.Get[Position](entity)
if err != nil {
    // Handle error
}

// Remove components
world.Remove[Position](entity)

// Destroy entities
world.Destroy(entity)
```
