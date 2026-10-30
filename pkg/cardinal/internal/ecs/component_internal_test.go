package ecs

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing component registration
// -------------------------------------------------------------------------------------------------
// This test verifies the archetype implementation correctness by applying random sequences of
// operations and comparing it against a regular Go map of name->id as the model. We also verify
// structural invariants: name-id bijection and component id uniqueness.
// -------------------------------------------------------------------------------------------------

func TestComponent_RegisterModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax     = 1 << 15 // 32_768 iterations
		opRegister = "register"
		opGetID    = "getID"
	)

	// Randomize operation weights.
	operations := []string{opRegister, opGetID}
	weights := testutils.RandOpWeights(prng, operations)

	impl := newComponentManager()
	model := make(map[string]ComponentID) // name -> cid

	for range opsMax {
		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opRegister:
			name := randValidComponentName(prng)

			implID, implErr := impl.register[testutils.SimpleComponent](name)
			modelID, modelExists := model[name]

			if modelExists {
				// Property: re-registering returns same ID.
				require.NoError(t, implErr)
				assert.Equal(t, modelID, implID, "re-register(%s) ID mismatch", name)
			} else {
				// Property: new registration succeeds and assigns next ID.
				require.NoError(t, implErr)
				model[name] = implID
			}

		case opGetID:
			// Bias toward registered names (80%) to test retrieval path.
			var name string
			if len(model) > 0 && prng.Float64() < 0.8 {
				name = testutils.RandMapKey(prng, model)
			} else {
				name = randValidComponentName(prng)
			}

			implID, implErr := impl.getID(name)
			modelID, modelExists := model[name]

			// Property: getID returns same existence and value as model.
			assert.Equal(t, modelExists, implErr == nil, "getID(%s) existence mismatch", name)
			if modelExists {
				assert.Equal(t, modelID, implID, "getID(%s) ID mismatch", name)
			}

		default:
			panic("unreachable")
		}
	}

	// Property: bijection holds between names and IDs.
	// Bijection means there's a 1-1 mapping of name->ID. Every name maps to a unique ID, and
	// every ID comes from a unique name.
	seenIDs := make(map[ComponentID]string)
	for name, id := range impl.catalog {
		if prevName, seen := seenIDs[id]; seen {
			t.Errorf("ID %d is mapped by both %q and %q", id, prevName, name)
		}
		seenIDs[id] = name
	}

	// Property: all IDs in catalog are in range [0, nextID).
	for name, id := range impl.catalog {
		assert.Less(t, id, impl.nextID, "ID for %q is out of range", name)
	}

	// Final state check: every model entry exists in impl with matching ID.
	assert.Len(t, impl.catalog, len(model), "catalog length mismatch")
	for name, modelID := range model {
		implID, err := impl.getID(name)
		require.NoError(t, err, "component %q in model but not in impl", name)
		assert.Equal(t, modelID, implID, "component %q ID mismatch", name)
	}

	// Simple test to confirm that registering the same name repeatedly is a no-op.
	t.Run("registration idempotence", func(t *testing.T) {
		t.Parallel()

		cm := newComponentManager()

		id1, err := cm.register[testutils.SimpleComponent]("hello")
		require.NoError(t, err)

		id2, err := cm.register[testutils.SimpleComponent]("hello")
		require.NoError(t, err)

		assert.Equal(t, id1, id2)

		id3, err := cm.register[testutils.SimpleComponent]("a_different_name")
		require.NoError(t, err)

		assert.Equal(t, id1+1, id3)
	})
}

// -------------------------------------------------------------------------------------------------
// Component name validation fuzz
// -------------------------------------------------------------------------------------------------
// This test verifies validateComponentName correctly implements the expr identifier specification:
// identifiers must start with [a-zA-Z_] and contain only [a-zA-Z0-9_].
// -------------------------------------------------------------------------------------------------

func TestComponent_NameValidationFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	valid, invalid := 0, 0

	for range opsMax {
		// Generate valid names + invalid names (by corrupting valid names).
		// We're using a custom generator instead of Go's builtin testing/quick because the latter
		// generates purely random string which doesn't exercise the validation logic much.
		b := []byte(randValidComponentName(prng))

		if prng.Float64() < 0.95 && len(b) > 0 {
			numFlips := prng.IntN(5) + 1
			for range numFlips {
				idx := prng.IntN(len(b))
				bit := uint8(1 << prng.IntN(8))
				b[idx] ^= bit
			}
		}

		name := string(b)
		expected := assertNameProperties(name)
		actual := validateComponentName(name) == nil
		assert.Equal(t, expected, actual, "mismatch for name: %q", name)

		if actual {
			valid++
		} else {
			invalid++
		}
	}
	assert.Equal(t, opsMax, valid+invalid)
}

func assertNameProperties(name string) bool {
	// Property: name cannot be empty.
	if name == "" {
		return false
	}
	// Property: name must start with a letter or underscore.
	first := name[0]
	if (first < 'a' || first > 'z') && (first < 'A' || first > 'Z') && first != '_' {
		return false
	}
	// Property: name can only contain alphanumeric characters and underscore.
	for i := 1; i < len(name); i++ {
		c := name[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

func randValidComponentName(prng *rand.Rand) string {
	const firstChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_"
	const restChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
	length := prng.IntN(100) + 1 // 1-100 characters
	b := make([]byte, length)
	b[0] = firstChars[prng.IntN(len(firstChars))]
	for i := 1; i < length; i++ {
		b[i] = restChars[prng.IntN(len(restChars))]
	}
	return string(b)
}

type conflictingComponent struct {
	testutils.SimpleComponent
}

func TestWorld_HasComponent(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	eid := w.Create()
	assert.False(t, w.Has[testutils.ComponentA](eid), "unregistered component")

	_, err := w.RegisterComponent[testutils.ComponentA]()
	require.NoError(t, err)
	assert.False(t, w.Has[testutils.ComponentA](eid), "registered but absent component")

	require.NoError(t, w.Set(eid, testutils.ComponentA{X: 42}))
	assert.True(t, w.Has[testutils.ComponentA](eid), "present component")

	require.NoError(t, w.Remove[testutils.ComponentA](eid))
	assert.False(t, w.Has[testutils.ComponentA](eid), "removed component")

	require.NoError(t, w.Set(eid, testutils.ComponentA{X: 42}))
	require.True(t, w.Destroy(eid))
	assert.False(t, w.Has[testutils.ComponentA](eid), "destroyed entity")
}

func TestWorld_RegisterComponentRejectsNameCollision(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	id, err := w.RegisterComponent[testutils.SimpleComponent]()
	require.NoError(t, err)
	require.Equal(t, ComponentID(0), id)
	eid := w.Create()
	require.NoError(t, w.Set(eid, testutils.SimpleComponent{Value: 42}))

	id, err = w.RegisterComponent[testutils.SimpleComponent]()
	require.NoError(t, err)
	require.Equal(t, ComponentID(0), id)
	_, err = w.RegisterComponent[conflictingComponent]()
	require.ErrorContains(t, err, "component simple_component already registered with a different type")
	assert.False(t, w.Has[conflictingComponent](eid), "a shared name does not register a different Go type")
	assert.True(t, w.Has[testutils.SimpleComponent](eid))

	value, err := w.Get[testutils.SimpleComponent](eid)
	require.NoError(t, err)
	require.Equal(t, testutils.SimpleComponent{Value: 42}, value)
	require.NoError(t, w.Set(eid, testutils.SimpleComponent{Value: 7}))
	value, err = w.Get[testutils.SimpleComponent](eid)
	require.NoError(t, err)
	require.Equal(t, testutils.SimpleComponent{Value: 7}, value)
}

func TestWorld_ConflictingComponentAccessPreservesData(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	_, err := w.RegisterComponent[testutils.SimpleComponent]()
	require.NoError(t, err)
	eid := w.Create()
	require.NoError(t, w.Set(eid, testutils.SimpleComponent{Value: 42}))

	require.ErrorIs(t, w.Remove[conflictingComponent](eid), ErrComponentNotFound)
	assert.False(t, w.Has[conflictingComponent](eid))
	_, err = w.Get[conflictingComponent](eid)
	require.ErrorIs(t, err, ErrComponentNotFound)
	err = w.Set(eid, conflictingComponent{testutils.SimpleComponent{Value: 99}})
	require.ErrorIs(t, err, ErrComponentNotFound)
	assert.False(t, w.Has[conflictingComponent](eid), "a type-conflicting Set must not register a different Go type")
	value, err := w.Get[testutils.SimpleComponent](eid)
	require.NoError(t, err)
	assert.Equal(t, testutils.SimpleComponent{Value: 42}, value)
}

// -------------------------------------------------------------------------------------------------
// Component type validation
// -------------------------------------------------------------------------------------------------
// A component is stored, copied by Get, and written to snapshots. These tests cover every field
// shape the SDK generator refused, which of them are still allowed, how the walk reaches nested
// fields, and that a refused type never registers.
// -------------------------------------------------------------------------------------------------

type valuePoint struct{ X, Y int }

type sliceHolder struct{ Items []int }

type hiddenOnly struct {
	n int //nolint:unused // Only its type is inspected.
}

type hiddenBesideExported struct {
	X     int
	items []int //nolint:unused // Only its type is inspected.
}

type embeddedSlice struct{ Items []int }

type chanHolder struct{ C chan int }

type selfViaImmutable struct {
	Children immutable.Slice[selfViaImmutable]
}

type sliceComponent []int

func (sliceComponent) Name() string { return "slice_component" }

type mapComponent struct{ Scores map[string]int }

func (mapComponent) Name() string { return "map_component" }

func TestComponent_ValidateTypeAllowsStorableShapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		typ  reflect.Type
	}{
		{"scalars", reflect.TypeFor[struct {
			B bool
			I int64
			U uint8
			F float32
		}]()},
		{"string", reflect.TypeFor[struct{ S string }]()},
		{"fixed array", reflect.TypeFor[struct{ A [4]int }]()},
		{"fixed byte array", reflect.TypeFor[struct{ A [32]byte }]()},
		{"multi-dimensional array", reflect.TypeFor[struct{ A [2][3]int }]()},
		{"array of structs", reflect.TypeFor[struct{ A [2]valuePoint }]()},
		{"nested struct", reflect.TypeFor[struct{ P valuePoint }]()},
		{"time.Time", reflect.TypeFor[struct{ T time.Time }]()},
		{"immutable.Slice of scalars", reflect.TypeFor[struct{ S immutable.Slice[int] }]()},
		{"immutable.Slice of structs", reflect.TypeFor[struct{ S immutable.Slice[valuePoint] }]()},
		{"immutable.Slice of arrays", reflect.TypeFor[struct{ S immutable.Slice[[4]byte] }]()},
		{"immutable.Slice of immutable.Slice", reflect.TypeFor[struct {
			S immutable.Slice[immutable.Slice[int]]
		}]()},
		{"type reaching itself through immutable.Slice", reflect.TypeFor[selfViaImmutable]()},
		{"unexported field beside exported ones", reflect.TypeFor[hiddenBesideExported]()},
		{"empty struct", reflect.TypeFor[struct{ E struct{} }]()},
		{"scalar component", reflect.TypeFor[testutils.SimpleComponent]()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, validateComponentType(tc.typ))
		})
	}
}

func TestComponent_ValidateTypeRefusesUnstorableFields(t *testing.T) {
	t.Parallel()

	const (
		shared      = "shares memory with the stored component"
		notData     = "is not data"
		unsupported = "cannot be encoded in a snapshot"
	)
	cases := []struct {
		name string
		typ  reflect.Type
		want []string // one error per entry, in field order: "path (type) reason"
	}{
		// Shared memory.
		{"slice", reflect.TypeFor[sliceHolder](), []string{"sliceHolder.Items ([]int) " + shared}},
		{"byte slice", reflect.TypeFor[struct{ B []byte }](), []string{".B ([]uint8) " + shared}},
		{"map", reflect.TypeFor[mapComponent](), []string{"mapComponent.Scores (map[string]int) " + shared}},
		{"pointer", reflect.TypeFor[struct{ P *int }](), []string{".P (*int) " + shared}},
		{"pointer to struct", reflect.TypeFor[struct{ P *valuePoint }](), []string{".P (*ecs.valuePoint) " + shared}},
		{"empty interface", reflect.TypeFor[struct{ V any }](), []string{".V (interface {}) " + shared}},
		{"component that is itself a slice", reflect.TypeFor[sliceComponent](),
			[]string{"sliceComponent (ecs.sliceComponent) " + shared}},

		// Not data.
		{"channel", reflect.TypeFor[struct{ C chan int }](), []string{".C (chan int) " + notData}},
		{"function", reflect.TypeFor[struct{ F func() }](), []string{".F (func()) " + notData}},
		{"unsafe.Pointer", reflect.TypeFor[struct{ P unsafe.Pointer }](), []string{".P (unsafe.Pointer) " + notData}},
		{"method interface", reflect.TypeFor[struct{ S fmt.Stringer }](), []string{".S (fmt.Stringer) " + notData}},
		{"struct with no exported fields", reflect.TypeFor[struct{ H hiddenOnly }](),
			[]string{".H (ecs.hiddenOnly) " + notData}},
		{"sync.Mutex", reflect.TypeFor[struct{ M sync.Mutex }](), []string{".M (sync.Mutex) " + notData}},
		{"slice of channels names the channel", reflect.TypeFor[struct{ C []chan int }](),
			[]string{".C ([]chan int) " + notData}},
		{"map of functions names the function", reflect.TypeFor[struct{ M map[string]func() }](),
			[]string{".M (map[string]func()) " + notData}},
		{"pointer to a struct holding a channel", reflect.TypeFor[struct{ P *chanHolder }](),
			[]string{".P (*ecs.chanHolder) " + notData}},

		// Unsupported.
		{"uintptr", reflect.TypeFor[struct{ U uintptr }](), []string{".U (uintptr) " + unsupported}},
		{"complex64", reflect.TypeFor[struct{ C complex64 }](), []string{".C (complex64) " + unsupported}},
		{"complex128", reflect.TypeFor[struct{ C complex128 }](), []string{".C (complex128) " + unsupported}},

		// Where the walk reaches.
		{"slice inside a fixed array", reflect.TypeFor[struct{ A [4][]int }](), []string{".A[] ([]int) " + shared}},
		{
			"channel inside a fixed array",
			reflect.TypeFor[struct{ A [4]chan int }](),
			[]string{".A[] (chan int) " + notData},
		},
		{
			"slice inside a nested struct",
			reflect.TypeFor[struct{ H sliceHolder }](),
			[]string{".H.Items ([]int) " + shared},
		},
		{"slice inside an array of structs", reflect.TypeFor[struct{ A [2]sliceHolder }](),
			[]string{".A[].Items ([]int) " + shared}},
		{"slice inside immutable.Slice", reflect.TypeFor[struct{ S immutable.Slice[[]int] }](),
			[]string{".S[] ([]int) " + shared}},
		{"pointer inside immutable.Slice", reflect.TypeFor[struct{ S immutable.Slice[*int] }](),
			[]string{".S[] (*int) " + shared}},
		{"function inside immutable.Slice", reflect.TypeFor[struct{ S immutable.Slice[func()] }](),
			[]string{".S[] (func()) " + notData}},
		{"slice inside a struct inside immutable.Slice", reflect.TypeFor[struct{ S immutable.Slice[sliceHolder] }](),
			[]string{".S[].Items ([]int) " + shared}},
		{"exported field of an unexported embedded struct", reflect.TypeFor[struct{ embeddedSlice }](),
			[]string{".embeddedSlice.Items ([]int) " + shared}},
		{
			"every offending field is reported",
			reflect.TypeFor[struct {
				A []int
				B int
				C chan int
				D complex64
			}](),
			[]string{".A ([]int) " + shared, ".C (chan int) " + notData, ".D (complex64) " + unsupported},
		},
		{
			"the same nested type is reported at each field that holds it",
			reflect.TypeFor[struct{ First, Second sliceHolder }](),
			[]string{".First.Items ([]int) " + shared, ".Second.Items ([]int) " + shared},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateComponentType(tc.typ)
			require.Error(t, err)
			joined, ok := err.(interface{ Unwrap() []error })
			require.True(t, ok, "errors are joined, one per field")
			errs := joined.Unwrap()
			require.Len(t, errs, len(tc.want), "got: %v", err)
			for i, want := range tc.want {
				assert.Contains(t, errs[i].Error(), want)
			}
		})
	}
}

func TestWorld_RegisterComponentRejectsUnstorableFields(t *testing.T) {
	t.Parallel()
	w := NewWorld()

	_, err := w.RegisterComponent[mapComponent]()
	require.ErrorContains(t, err, "component map_component")
	require.ErrorContains(t, err, "mapComponent.Scores (map[string]int)")
	_, err = w.RegisterComponent[sliceComponent]()
	require.ErrorContains(t, err, "component slice_component")

	// A refused type takes no ID, so the next valid one still gets the first.
	_, lookupErr := w.state.components.getID("map_component")
	require.ErrorIs(t, lookupErr, ErrComponentNotFound)
	id, err := w.RegisterComponent[testutils.SimpleComponent]()
	require.NoError(t, err)
	assert.Equal(t, ComponentID(0), id)
}
