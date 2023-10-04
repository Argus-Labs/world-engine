package component

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"unsafe"

	"pkg.world.dev/world-engine/cardinal/ecs/world_namespace"

	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/component_types"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/icomponent"
	"pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/query"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
)

type IGettableRawJsonFromEntityId interface {
	GetRawJson(s *store.Manager, id entity.ID) (json.RawMessage, error)
}

// NewComponentType creates a new component type.
// The function is used to create a new component of the type.
func NewComponentType[T any](name string, opts ...ComponentOption[T]) *ComponentType[T] {
	var t T
	comp := newComponentType(t, name, nil)
	for _, opt := range opts {
		opt(comp)
	}
	return comp
}

// ComponentType represents a type of component. It is used to identify
// a component when getting or setting the component of an entity.
type ComponentType[T any] struct {
	isIDSet    bool
	id         component_types.TypeID
	typ        reflect.Type
	name       string
	defaultVal interface{}
	query      *query.Query
}

var _ IGettableRawJsonFromEntityId = &ComponentType[int]{}

// SetID set's this component's ID. It must be unique across the world object.
func (c *ComponentType[T]) SetID(id component_types.TypeID) error {
	if c.isIDSet {
		// In games implemented with Cardinal, components will only be initialized one time (on startup).
		// In tests, it's often useful to use the same component in multiple worlds. This check will allow for the
		// re-initialization of components, as long as the ID doesn't change.
		if id == c.id {
			return nil
		}
		return fmt.Errorf("id for component %v is already set to %v, cannot change to %v", c, c.id, id)
	}
	c.id = id
	c.isIDSet = true
	return nil
}

// Get returns component data from the entity.
func (c *ComponentType[T]) Get(s *store.Manager, id entity.ID) (comp T, err error) {
	value, err := s.GetComponentForEntity(c, id)
	if err != nil {
		return comp, err
	}
	comp, ok := value.(T)
	if !ok {
		return comp, fmt.Errorf("type assertion for component failed: %v to %v", value, c)
	}
	return comp, nil
}

func (c *ComponentType[T]) GetRawJson(s *store.Manager, id entity.ID) (json.RawMessage, error) {
	return s.GetComponentForEntityInRawJson(c, id)
}

// Update is a helper that combines a Get followed by a Set to modify a component's value. Pass in a function
// fn that will return a modified component. Update will hide the calls to Get and Set
func (c *ComponentType[T]) Update(logger *log.Logger, nameToComponent map[string]icomponent.IComponentType, s *store.Manager, id entity.ID, fn func(T) T) error {
	if _, ok := nameToComponent[c.Name()]; !ok {
		return fmt.Errorf("%s is not registered, please register it before updating", c.Name())
	}
	val, err := c.Get(s, id)
	if err != nil {
		return err
	}
	val = fn(val)
	return c.Set(logger, nameToComponent, s, id, val)
}

// Set sets component data to the entity.
func (c *ComponentType[T]) Set(logger *log.Logger, nameToComponent map[string]icomponent.IComponentType, s *store.Manager, id entity.ID, component T) error {
	if _, ok := nameToComponent[c.Name()]; !ok {
		return fmt.Errorf("%s is not registered, please register it before updating", c.Name())
	}
	err := s.SetComponentForEntity(c, id, component)
	if err != nil {
		return err
	}
	logger.Logger.Debug().
		Str("entity_id", strconv.FormatUint(uint64(id), 10)).
		Str("component_name", c.name).
		Int("component_id", int(c.ID())).
		Msg("entity updated")
	return nil
}

// Each iterates over the entities that have the component.
// If you would like to stop the iteration, return false to the callback. To continue iterating, return true.
func (c *ComponentType[T]) Each(namespace string, worldStorage *storage.WorldStorage, callback query.QueryCallBackFn) {
	c.query.Each(world_namespace.Namespace(namespace), worldStorage, callback)
}

// First returns the first entity that has the component.
func (c *ComponentType[T]) First(namespace string, worldStorage *storage.WorldStorage) (entity.ID, error) {
	return c.query.First(namespace, worldStorage)
}

// MustFirst returns the first entity that has the component or panics.
func (c *ComponentType[T]) MustFirst(namespace string, worldStorage *storage.WorldStorage) entity.ID {
	id, err := c.query.First(namespace, worldStorage)
	if err != nil {
		panic(fmt.Sprintf("no entity has the component %s", c.name))
	}
	return id
}

// RemoveFrom removes this component from the given entity.
func (c *ComponentType[T]) RemoveFrom(s *store.Manager, id entity.ID) error {
	return s.RemoveComponentFromEntity(c, id)
}

// AddTo adds this component to the given entity.
func (c *ComponentType[T]) AddTo(s *store.Manager, id entity.ID) error {
	return s.AddComponentToEntity(c, id)
}

// String returns the component type name.
func (c *ComponentType[T]) String() string {
	return c.name
}

// SetName sets the component type name.
func (c *ComponentType[T]) SetName(name string) *ComponentType[T] {
	c.name = name
	return c
}

// Name returns the component type name.
func (c *ComponentType[T]) Name() string {
	return c.name
}

// ID returns the component type id.
func (c *ComponentType[T]) ID() component_types.TypeID {
	return c.id
}

func (c *ComponentType[T]) New() ([]byte, error) {
	var comp T
	if c.defaultVal != nil {
		comp = c.defaultVal.(T)
	}
	return codec.Encode(comp)
}

func (c *ComponentType[T]) Encode(v any) ([]byte, error) {
	return codec.Encode(v)
}

func (c *ComponentType[T]) Decode(bz []byte) (any, error) {
	return codec.Decode[T](bz)
}

func (c *ComponentType[T]) setDefaultVal(ptr unsafe.Pointer) {
	v := reflect.Indirect(reflect.ValueOf(c.defaultVal))
	reflect.NewAt(c.typ, ptr).Elem().Set(v)
}

func (c *ComponentType[T]) validateDefaultVal() {
	if !reflect.TypeOf(c.defaultVal).AssignableTo(c.typ) {
		err := fmt.Sprintf("default value is not assignable to component type: %s", c.name)
		panic(err)
	}
}

// newComponentType creates a new component type.
// The argument is a struct that represents a data of the component.
func newComponentType[T any](s T, name string, defaultVal interface{}) *ComponentType[T] {
	componentType := &ComponentType[T]{
		typ:        reflect.TypeOf(s),
		name:       name,
		defaultVal: defaultVal,
	}
	componentType.query = query.NewQuery(filter.Contains(componentType))
	if defaultVal != nil {
		componentType.validateDefaultVal()
	}
	return componentType
}

// ComponentOption is a type that can be passed to NewComponentType to augment the creation
// of the component type
type ComponentOption[T any] func(c *ComponentType[T])

// WithDefault updated the created ComponentType with a default value
func WithDefault[T any](defaultVal T) ComponentOption[T] {
	return func(c *ComponentType[T]) {
		c.defaultVal = defaultVal
		c.validateDefaultVal()
	}
}
