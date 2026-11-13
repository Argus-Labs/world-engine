package sync_test

import (
	"reflect"
	"testing"

	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

func TestMarkerDoesNotChangeMessagePack(t *testing.T) {
	type private struct {
		HP    int
		MaxHP int
	}
	type public struct {
		syncstate.State

		HP    int
		MaxHP int
	}
	before, err := msgpack.Marshal(private{HP: 7, MaxHP: 100})
	require.NoError(t, err)
	after, err := msgpack.Marshal(public{HP: 7, MaxHP: 100})
	require.NoError(t, err)
	require.Equal(t, before, after)
	_, exposed, err := syncstate.Describe(reflect.TypeFor[private](), "health")
	require.NoError(t, err)
	require.False(t, exposed)
	component, exposed, err := syncstate.Describe(reflect.TypeFor[public](), "health")
	require.NoError(t, err)
	require.True(t, exposed)
	require.Equal(t, []syncstate.Field{{Name: "HP", Type: "int64"}, {Name: "MaxHP", Type: "int64"}}, component.Fields)
}

func TestContractPinsWidthsAndNames(t *testing.T) {
	type health struct {
		syncstate.State

		HP      int32 `msgpack:"hp"`
		Speed   float32
		Private int `msgpack:"-"`
	}
	c, _, err := syncstate.Describe(reflect.TypeFor[health](), "health")
	require.NoError(t, err)
	require.Equal(t, []syncstate.Field{{Name: "Speed", Type: "float32"}, {Name: "hp", Type: "int32"}}, c.Fields)
	a := syncstate.Contract{
		Version:    1,
		Components: []syncstate.Component{c, {Name: "other", Fields: []syncstate.Field{{Name: "x", Type: "int64"}}}},
	}
	b := syncstate.Contract{Version: 1, Components: []syncstate.Component{a.Components[1], c}}
	af, err := a.Fingerprint()
	require.NoError(t, err)
	bf, err := b.Fingerprint()
	require.NoError(t, err)
	require.Equal(t, af, bf)
	b.Components[1].Fields[0].Type = "float64"
	bf, err = b.Fingerprint()
	require.NoError(t, err)
	require.NotEqual(t, af, bf)
}

func TestRejectUnsupportedSyncShapes(t *testing.T) {
	for _, value := range []any{
		struct {
			syncstate.State

			Values []int
		}{}, struct {
			syncstate.State

			Value int `msgpack:",omitempty"`
		}{},
		struct {
			syncstate.State

			A int `msgpack:"x"`
			B int `msgpack:"x"`
		}{},
	} {
		_, exposed, err := syncstate.Describe(reflect.TypeOf(value), "state")
		require.True(t, exposed)
		require.Error(t, err)
	}
}
