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
├── EntityManager
│   ├── Active Entities ([]EntityID)
│   ├── Sparse Index ([]int)
│   ├── Recycled IDs ([]EntityID)
│   └── Entity -> Archetype Map (map[EntityID]*archetype)
│
└── Archetypes ([]archetype)
    └── archetype
       ├── Entity Bitmap
       ├── Component Type Bitmap
       └── Columns ([]any)
           ├── Column<T1>
           │   ├── Sparse Index ([]int)
           │   ├── Dense Array ([]EntityID)
           │   └── Data Array ([]T1)
           └── Column<T2>
               ├── Sparse Index ([]int)
               ├── Dense Array ([]EntityID)
               └── Data Array ([]T2)
```

### Core Components

1. **World (`World`)**:
   - The root container managing all ECS state
   - Handles component type registration
   - Manages entity lifecycle and component operations
   - Maintains archetype organization

2. **Entities (`EntityManager`)**:
   - Lightweight numeric identifiers (EntityID)
   - Uses a sparse set pattern for efficient ID management
   - O(1) operations for creation, deletion, and lookups
   - Supports ID recycling for memory efficiency
   - Maximum of 2^31-1 entities for bitmap compatibility

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
   - Type-safe component storage containers
   - Uses sparse set data structure
   - O(1) component access and modification
   - Memory-efficient for sparse data
   - Aligned with cache-friendly access patterns

### Key Design Features

1. **Sparse Set Pattern**:
   - Used in both entity and component management
   - Provides O(1) operations for common tasks
   - Enables efficient iteration over active elements
   - Reduces memory fragmentation
   - Supports fast component access

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

## Usage Example

```go
// Define a component
type Position struct{ X, Y float32 }
func (Position) Name() string { return "Position" }

// Create a world and register components
world := ecs.NewWorld()
ecs.RegisterComponent[Position](world)

// Create an entity with components
entity := ecs.Create(world, Position{X: 1, Y: 2})

// Modify components
ecs.Set(world, entity, Position{X: 3, Y: 4})

// Query components
pos, err := ecs.Get[Position](world, entity)
if err != nil {
    // Handle error
}

// Remove components
ecs.Remove[Position](world, entity)

// Destroy entities
ecs.Destroy(world, entity)
```
