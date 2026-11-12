package ecs

import (
	"errors"
	"math"
	"net/netip"
	"reflect"
	"regexp"
	"slices"
	"strings"
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
// the rules the SDK generator enforced on wire types, minus the ones that only protobuf needed, plus
// the ones a type with its own MessagePack codec needs.
func validateComponentType(t reflect.Type) error {
	var problems []fieldProblem
	checkComponentField(t, t.Name(), map[reflect.Type]bool{}, &problems)
	errs := make([]error, len(problems))
	for i, p := range problems {
		errs[i] = eris.Errorf("%s (%s) %s; %s", p.path, p.typ, p.reason, p.fix)
	}
	return errors.Join(errs...)
}

// fieldProblem is a field that cannot be stored, why, and how to fix it.
type fieldProblem struct {
	path        string
	typ         reflect.Type
	reason, fix string
}

// checkComponentField walks t and appends one problem per field that cannot be stored. visiting holds
// the types on the current path, so a type that reaches itself is reported once instead of looping.
func checkComponentField(t reflect.Type, path string, visiting map[reflect.Type]bool, problems *[]fieldProblem) {
	if visiting[t] {
		*problems = append(*problems, fieldProblem{path: path, typ: t, reason: reasonRecursive, fix: fixRecursive})
		return
	}
	if knownValueType(t) {
		return
	}
	visiting[t] = true
	defer delete(visiting, t)
	report := func(reason, fix string) {
		*problems = append(*problems, fieldProblem{path: path, typ: t, reason: reason, fix: fix})
	}

	if elem, ok := immutable.SliceElem(t); ok {
		// The Slice itself is safe; its element might not be.
		checkComponentField(elem, path+"[]", visiting, problems)
		return
	}
	if key, value, ok := immutable.MapTypes(t); ok {
		// Key already limits the key to an integer or string, but a named one can still carry a codec.
		checkComponentField(key, path+"{key}", visiting, problems)
		checkComponentField(value, path+"{}", visiting, problems)
		return
	}
	if hasCodec(t) {
		if f, ok := embeddedCodec(t); ok {
			embedded := fieldProblem{
				path:   path + "." + f.Name,
				typ:    f.Type,
				reason: reasonEmbeddedCodec,
				fix:    fixEmbeddedCodec,
			}
			*problems = append(*problems, embedded)
		} else if reason, fix := codecProblem(t); reason != "" {
			report(reason, fix)
		}
		return
	}
	switch t.Kind() { //nolint:exhaustive // Scalars and strings are stored as they are.
	case reflect.Array:
		checkComponentField(t.Elem(), path+"[]", visiting, problems)
	case reflect.Struct:
		if hidesState(t) {
			report(reasonNotData, fixNotData)
			return
		}
		checkStoredFields(t, path, visiting, problems)
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface:
		// A collection of something that is not data needs that fixed first, so name it instead.
		if holdsNotData(t, visiting) {
			report(reasonNotData, fixNotData)
			return
		}
		report(reasonSharedMemory, sharedMemoryFix(t.Kind()))
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		report(reasonNotData, fixNotData)
	case reflect.Uintptr, reflect.Complex64, reflect.Complex128:
		report(reasonUnsupported, fixUnsupported)
	}
}

// checkStoredFields checks each field of struct t that a snapshot stores.
func checkStoredFields(t reflect.Type, path string, visiting map[reflect.Type]bool, problems *[]fieldProblem) {
	for _, f := range storedFields(t) {
		before := len(*problems)
		checkComponentField(f.Type, path+"."+f.Name, visiting, problems)
		// MessagePack stores an unexported embedded non-struct under its type name, but a restore
		// cannot write to it. An embedded struct is fine: its exported fields are written directly.
		if len(*problems) == before && f.Anonymous && !f.IsExported() && f.Type.Kind() != reflect.Struct {
			unexported := fieldProblem{
				path:   path + "." + f.Name,
				typ:    f.Type,
				reason: reasonUnexportedEmbedded,
				fix:    fixUnexportedEmbedded,
			}
			*problems = append(*problems, unexported)
		}
	}
}

// holdsNotData reports whether a slice, map, pointer or interface type holds something that is not
// data: a method interface, or anything the walk would report as not data.
func holdsNotData(t reflect.Type, visiting map[reflect.Type]bool) bool {
	var inner []fieldProblem
	switch t.Kind() { //nolint:exhaustive // Called only for the kinds below.
	case reflect.Interface:
		return t.NumMethod() > 0
	case reflect.Map:
		checkComponentField(t.Key(), "", visiting, &inner)
		checkComponentField(t.Elem(), "", visiting, &inner)
	default:
		checkComponentField(t.Elem(), "", visiting, &inner)
	}
	return slices.ContainsFunc(inner, func(p fieldProblem) bool { return p.reason == reasonNotData })
}

// hasCodec reports whether t encodes or decodes itself rather than field by field. A pointer or
// interface only forwards to what it holds, so the walk judges those by their kind.
func hasCodec(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer || t.Kind() == reflect.Interface {
		return false
	}
	return schema.EncodesItself(t) || schema.EncodesOnlyByPointer(t) || schema.DecodesItself(t)
}

// codecProblem returns why a type with its own codec cannot be stored and how to fix it, or empty
// strings if it can. Its codec decides what a snapshot holds, so its fields are not walked; it only
// has to be callable on a copy, read back what it wrote, and own its state.
func codecProblem(t reflect.Type) (string, string) {
	switch {
	case schema.EncodesOnlyByPointer(t):
		return reasonPointerEncoder, fixPointerEncoder
	case schema.EncodesItself(t) != schema.DecodesItself(t):
		return reasonOneSidedCodec, fixOneSidedCodec
	}
	if !plainValue(t) {
		return reasonCodecShares, fixCodecShares
	}
	return "", ""
}

// embeddedCodec returns an embedded field that supplies a method msgpack calls to encode or decode t.
// Go promotes the embedded type's methods, so the struct is then stored as that embedded value alone
// and drops any field beside it; that holds with nothing beside it too, until a field is added. A
// struct that declares its own codec hides the promoted methods and is judged like any codec type.
// When it declares the same method the embedded type has, reflection cannot tell the two apart, so
// that is refused as well.
func embeddedCodec(t reflect.Type) (reflect.StructField, bool) {
	if t.Kind() != reflect.Struct {
		return reflect.StructField{}, false
	}
	enc, encodes := schema.EncoderMethod(t)
	dec, decodes := schema.DecoderMethod(t)
	for f := range t.Fields() {
		if !f.Anonymous {
			continue
		}
		if (encodes && hasMethod(f.Type, enc)) || (decodes && hasMethod(reflect.PointerTo(f.Type), dec)) {
			return f, true
		}
	}
	return reflect.StructField{}, false
}

// hasMethod reports whether t has the named method. A pointer to a pointer has no methods, so it
// looks one level down: an embedded *S supplies the methods of *S.
func hasMethod(t reflect.Type, name string) bool {
	if t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Pointer {
		t = t.Elem()
	}
	_, ok := t.MethodByName(name)
	return ok
}

// plainValue reports whether t holds only values, so a copy of it shares nothing with the original.
// Unexported fields count too: a codec type keeps its state in them.
func plainValue(t reflect.Type) bool {
	if knownValueType(t) {
		return true
	}
	switch t.Kind() { //nolint:exhaustive // Every other kind holds a reference or an address.
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return true
	case reflect.Array:
		return plainValue(t.Elem())
	case reflect.Struct:
		for f := range t.Fields() {
			if !plainValue(f.Type) {
				return false
			}
		}
		return true
	}
	return false
}

// knownValueType reports whether t is a standard library value type that encodes itself and is safe
// to copy. Each holds a pointer, but only to data that never changes, which plainValue cannot see.
func knownValueType(t reflect.Type) bool {
	switch t {
	case reflect.TypeFor[time.Time](), reflect.TypeFor[netip.Addr](),
		reflect.TypeFor[netip.AddrPort](), reflect.TypeFor[netip.Prefix]():
		return true
	}
	return false
}

func sharedMemoryFix(k reflect.Kind) string {
	switch k { //nolint:exhaustive // Called only for the shared-memory kinds.
	case reflect.Slice:
		return fixSlice
	case reflect.Map:
		return fixMap
	case reflect.Pointer:
		return fixPointer
	}
	return fixInterface
}

const (
	reasonSharedMemory   = "shares memory with the stored component, so writing into it changes the world without Set"
	reasonNotData        = "is not data, so a snapshot cannot store it"
	reasonUnsupported    = "cannot be encoded in a snapshot"
	reasonPointerEncoder = "has its encoding method only on a pointer, so a snapshot cannot call it on a stored copy"
	reasonOneSidedCodec  = "encodes or decodes itself but not both, so a restore cannot read back what a snapshot wrote"
	reasonEmbeddedCodec  = "is embedded, so its own codec stores the whole struct as that one value and drops any " +
		"field beside it"
	reasonCodecShares = "encodes itself but holds a reference, so a copy from Get shares it with the " +
		"stored component"
	reasonUnexportedEmbedded = "is embedded with an unexported type, so a snapshot stores it but a restore " +
		"cannot write it back"
	reasonRecursive = "contains itself, so the component is a tree with no bound on its depth"

	fixSlice = "use a fixed-size array [N]T with the live length in its own field, or immutable.Slice[T] " +
		"if the length is genuinely unbounded"
	fixMap       = "if the key set is fixed, use a struct with one field per key; otherwise immutable.Map[K, V]"
	fixPointer   = "drop the star and store the value; if it means absent, put a Has<Name> bool beside it"
	fixInterface = "use the one concrete type it holds; if it varies, add a tag field and one field per " +
		"variant"
	fixNotData     = "store an ID or enum that the code can rebuild the behavior from"
	fixUnsupported = "store a scalar, string, or struct of those; a complex number becomes two float64 " +
		"fields, and an address means nothing after a restore"
	fixPointerEncoder     = "give the encoding method a value receiver; the decoding method keeps its pointer receiver"
	fixOneSidedCodec      = "implement the matching method too, such as DecodeMsgpack beside EncodeMsgpack"
	fixEmbeddedCodec      = "give it a field name"
	fixCodecShares        = "keep its state in numbers, strings, fixed-size arrays, or structs of those"
	fixUnexportedEmbedded = "give it an exported field name, or export its type"
	fixRecursive          = "make each node its own entity with a parent component, or flatten the tree into " +
		"one immutable.Slice of nodes that refer to each other by index"
)

// hidesState reports whether t keeps state in unexported fields and stores none of it. A struct
// whose fields are only tagged msgpack:"-" stores as an empty map, which loses nothing it meant to keep.
func hidesState(t reflect.Type) bool {
	if len(storedFields(t)) > 0 {
		return false
	}
	for f := range t.Fields() {
		if !f.IsExported() && !f.Anonymous {
			return true
		}
	}
	return false
}

// storedFields returns the fields of t that a snapshot stores. MessagePack skips fields tagged
// `msgpack:"-"` and unexported fields, except that an embedded struct's exported fields are promoted
// and stored.
func storedFields(t reflect.Type) []reflect.StructField {
	var fields []reflect.StructField
	for f := range t.Fields() {
		skipped := strings.Split(f.Tag.Get("msgpack"), ",")[0] == "-"
		if (f.IsExported() || f.Anonymous) && !skipped {
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
