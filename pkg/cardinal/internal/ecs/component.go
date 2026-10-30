package ecs

import (
	"errors"
	"math"
	"reflect"
	"regexp"
	"slices"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/rotisserie/eris"
)

// Component is the interface that all components must implement.
// Components are pure data containers that can be attached to entities.
type Component interface {
	// Name returns a unique string identifier for the component type.
	// This should be consistent across program executions.
	//
	// Component names must follow these rules:
	//   - Start with a letter (a-z, A-Z) or underscore (_)
	//   - Contain only letters, digits (0-9), and underscores
	//   - Cannot contain hyphens (-), spaces, dots (.), or other special characters
	//
	// Valid examples: "Health", "PlayerData", "player_health", "_internal", "Component123"
	// Invalid examples: "player-data", "123Invalid", "my.component", "has space"
	//
	// These rules ensure component names work correctly in query expressions.
	schema.Serializable
}

// ComponentID is a unique identifier for a component type.
// It is used internally to track and manage component types efficiently.
type ComponentID = uint32

// maxComponentID is the maximum number of component types that can be registered.
const maxComponentID = math.MaxUint32 - 1

// componentManager manages component type registration and lookup.
type componentManager struct {
	nextID    ComponentID            // The next available component ID
	catalog   map[string]ComponentID // Component name -> component ID
	factories []columnFactory        // Component ID -> column factory
	names     []string               // Component ID -> name
	types     []reflect.Type         // Component ID -> registered Go type
}

// newComponentManager creates a new component manager.
func newComponentManager() componentManager {
	return componentManager{
		nextID:    0,
		catalog:   make(map[string]ComponentID),
		factories: make([]columnFactory, 0),
		names:     make([]string, 0),
		types:     make([]reflect.Type, 0),
	}
}

var componentNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// validateComponentName validates that a component name follows expr identifier rules.
// See: https://expr-lang.org/docs/language-definition#variables
func validateComponentName(name string) error {
	if name == "" {
		return eris.New("component name cannot be empty")
	}

	if !componentNamePattern.MatchString(name) {
		return eris.Errorf(
			"component name '%s' is invalid: must start with a letter or underscore, "+
				"and contain only letters, digits, and underscores",
			name,
		)
	}

	return nil
}

// validateComponentType reports every exported field of a component that cannot be stored. These are
// the rules the SDK generator enforced on wire types, minus the ones that only protobuf needed.
func validateComponentType(t reflect.Type) error {
	var errs []error
	checkComponentField(t, t.Name(), map[reflect.Type]bool{}, &errs)
	return errors.Join(errs...)
}

// checkComponentField walks t and appends one error per field that cannot be stored. visiting holds the
// types on the current path, so a type that reaches itself through an immutable.Slice does not loop.
func checkComponentField(t reflect.Type, path string, visiting map[reflect.Type]bool, errs *[]error) {
	if visiting[t] || t == reflect.TypeFor[time.Time]() { // time.Time encodes itself
		return
	}
	visiting[t] = true
	defer delete(visiting, t)

	if elem, ok := immutable.SliceElem(t); ok {
		// The Slice itself is safe; its element might not be.
		checkComponentField(elem, path+"[]", visiting, errs)
		return
	}
	switch t.Kind() { //nolint:exhaustive // Every other kind is checked by fieldProblem.
	case reflect.Array:
		checkComponentField(t.Elem(), path+"[]", visiting, errs)
	case reflect.Struct:
		fields := storedFields(t)
		if t.NumField() > 0 && len(fields) == 0 {
			*errs = append(*errs, eris.Errorf("%s (%s) %s; %s", path, t, reasonNotData, fixNotData))
			return
		}
		for _, f := range fields {
			checkComponentField(f.Type, path+"."+f.Name, visiting, errs)
		}
	default:
		if reason, fix := fieldProblem(t); reason != "" {
			*errs = append(*errs, eris.Errorf("%s (%s) %s; %s", path, t, reason, fix))
		}
	}
}

// fieldProblem returns why a field of type t cannot be stored and how to fix it, or empty strings if
// it can. Arrays and structs are walked by the caller instead.
func fieldProblem(t reflect.Type) (string, string) {
	// A collection of something that is not data needs that fixed first, so name it instead.
	if notData(t, map[reflect.Type]bool{}) {
		return reasonNotData, fixNotData
	}
	switch t.Kind() { //nolint:exhaustive // Scalars and strings are stored as they are.
	case reflect.Slice:
		return reasonSharedMemory, fixSlice
	case reflect.Map:
		return reasonSharedMemory, fixMap
	case reflect.Pointer:
		return reasonSharedMemory, fixPointer
	case reflect.Interface:
		return reasonSharedMemory, fixInterface
	case reflect.Uintptr, reflect.Complex64, reflect.Complex128:
		return reasonUnsupported, fixUnsupported
	}
	return "", ""
}

const (
	reasonSharedMemory = "shares memory with the stored component, so writing into it changes the world without Set"
	reasonNotData      = "is not data, so a snapshot cannot store it"
	reasonUnsupported  = "cannot be encoded in a snapshot"

	fixSlice = "use a fixed-size array [N]T with the live length in its own field, or immutable.Slice[T] " +
		"if the length is genuinely unbounded"
	fixMap = "if the key set is fixed, use a struct with one field per key; otherwise a fixed-size array " +
		"of entry structs plus a count"
	fixPointer   = "drop the star and store the value; if it means absent, put a Has<Name> bool beside it"
	fixInterface = "use the one concrete type it holds; if it varies, add a tag field and one field per " +
		"variant"
	fixNotData     = "store an ID or enum that the code can rebuild the behavior from"
	fixUnsupported = "store a scalar, string, or struct of those; a complex number becomes two float64 " +
		"fields, and an address means nothing after a restore"
)

// notData reports whether t holds a chan, func, [unsafe.Pointer], method interface, or struct with no
// exported fields anywhere inside it.
func notData(t reflect.Type, visiting map[reflect.Type]bool) bool {
	if visiting[t] || t == reflect.TypeFor[time.Time]() {
		return false
	}
	visiting[t] = true
	if elem, ok := immutable.SliceElem(t); ok {
		return notData(elem, visiting)
	}
	switch t.Kind() { //nolint:exhaustive // Other kinds are data.
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return true
	case reflect.Interface:
		return t.NumMethod() > 0
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return notData(t.Elem(), visiting)
	case reflect.Map:
		return notData(t.Key(), visiting) || notData(t.Elem(), visiting)
	case reflect.Struct:
		fields := storedFields(t)
		return (t.NumField() > 0 && len(fields) == 0) ||
			slices.ContainsFunc(fields, func(f reflect.StructField) bool { return notData(f.Type, visiting) })
	}
	return false
}

// storedFields returns the fields of t that a snapshot stores. MessagePack skips unexported fields,
// except that an embedded struct's exported fields are promoted and stored.
func storedFields(t reflect.Type) []reflect.StructField {
	var fields []reflect.StructField
	for f := range t.Fields() {
		if f.IsExported() || f.Anonymous {
			fields = append(fields, f)
		}
	}
	return fields
}

// register registers a new component type and returns its ID.
// Registering the same type again is a no-op. Reusing a name for another type returns an error.
func (cm *componentManager) register[T Component](name string) (ComponentID, error) {
	// Validate component name follows expr identifier rules
	if err := validateComponentName(name); err != nil {
		return 0, err
	}
	if err := validateComponentType(reflect.TypeFor[T]()); err != nil {
		return 0, eris.Wrapf(err, "component %s", name)
	}

	if cid, exists := cm.catalog[name]; exists {
		if cm.types[cid] != reflect.TypeFor[T]() {
			return 0, eris.Errorf("component %s already registered with a different type", name)
		}
		return cid, nil
	}

	if cm.nextID > maxComponentID {
		return 0, eris.New("max number of components exceeded")
	}

	schema.Register[T]()
	cm.catalog[name] = cm.nextID
	cm.factories = append(cm.factories, newColumnFactory[T]())
	cm.names = append(cm.names, name)
	cm.types = append(cm.types, reflect.TypeFor[T]())
	cm.nextID++
	assert.That(int(cm.nextID) == len(cm.factories), "component id doesn't match number of components")

	return cm.nextID - 1, nil
}

// getID returns a component's ID given a name.
func (cm *componentManager) getID(name string) (ComponentID, error) {
	id, exists := cm.catalog[name]

	if !exists {
		return 0, eris.Wrapf(ErrComponentNotFound, "component %s", name)
	}

	return id, nil
}

// lookup returns the ID of the registered component with the given name and Go type, or an
// error if it was never registered or its name is registered with a different type.
func (cm *componentManager) lookup(name string, typ reflect.Type) (ComponentID, error) {
	cid, err := cm.getID(name)
	if err != nil {
		return 0, err
	}
	if cm.types[cid] != typ {
		return 0, eris.Wrapf(ErrComponentNotFound, "component %s is registered with a different type", name)
	}
	return cid, nil
}

// RegisterComponent registers a component type with the world.
func (w *World) RegisterComponent[T Component]() (ComponentID, error) {
	var zero T
	if w.onComponentRegister != nil {
		if err := w.onComponentRegister(zero); err != nil {
			return 0, eris.Wrap(err, "component registered callback failed")
		}
	}
	return w.state.components.register[T](zero.Name())
}

// ComponentIDOf returns the ID of the registered component with c's name and dynamic type.
// It serves callers that hold a component value rather than a type parameter.
func (w *World) ComponentIDOf(c Component) (ComponentID, error) {
	return w.state.components.lookup(c.Name(), reflect.TypeOf(c))
}
