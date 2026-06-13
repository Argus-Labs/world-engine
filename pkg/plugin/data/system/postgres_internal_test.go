package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeReader is a configReader stub for exercising the Fetch contract without a database. rows holds
// array-mode payloads and singleton holds single-object payloads, both keyed by table; the reader
// picks between them on the singleton flag and records every call.
type fakeReader struct {
	rows       map[string][]byte
	singleton  map[string][]byte
	err        error
	calls      []string
	lastSingle bool
}

func (f *fakeReader) readTableJSON(_ context.Context, table string, singleton bool) ([]byte, error) {
	f.calls = append(f.calls, table)
	f.lastSingle = singleton
	if f.err != nil {
		return nil, f.err
	}
	if singleton {
		return f.singleton[table], nil
	}
	return f.rows[table], nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestTableFromFile(t *testing.T) {
	cases := map[string]string{
		"testdata/abilities.json": "abilities",
		"abilities.json":          "abilities",
		"data/mobs.json":          "mobs",
		"loot_table.json":         "loot_table",
	}
	for in, want := range cases {
		require.Equalf(t, want, tableFromFile(in), "tableFromFile(%q)", in)
	}
}

func TestPostgresSourceFetchReturnsRowsAndHash(t *testing.T) {
	body := []byte(`[{"id":"fireball","cooldown":3}]`)
	src := &PostgresSource{
		reader: &fakeReader{rows: map[string][]byte{"abilities": body}},
	}

	got, hash, err := src.Fetch(context.Background(), "testdata/abilities.json", "")

	require.NoError(t, err)
	require.Equal(t, body, got)
	require.Equal(t, sha256Hex(body), hash, "hash must be sha256 of the returned bytes, matching EmbedSource")
}

// Single-version: an unservable hash returns current rows (no error), so restore takes the warn
// path, not the panic path.
func TestPostgresSourceFetchIgnoresRequestedHash(t *testing.T) {
	body := []byte(`[{"id":"a"}]`)
	src := &PostgresSource{
		reader: &fakeReader{rows: map[string][]byte{"abilities": body}},
	}

	got, hash, err := src.Fetch(context.Background(), "abilities.json", "some-old-snapshot-hash")

	require.NoError(t, err, "an unservable historical hash must NOT error (single-version semantics)")
	require.Equal(t, body, got)
	require.Equal(t, sha256Hex(body), hash)
	require.NotEqual(t, "some-old-snapshot-hash", hash)
}

func TestPostgresSourceFetchResolvesTableFromFile(t *testing.T) {
	fr := &fakeReader{rows: map[string][]byte{"mobs": []byte("[]")}}
	src := &PostgresSource{reader: fr}

	_, _, err := src.Fetch(context.Background(), "config/mobs.json", "")

	require.NoError(t, err)
	require.Equal(t, []string{"mobs"}, fr.calls, "Fetch must query the table derived from the file name")
}

func TestPostgresSourceFetchWrapsReaderError(t *testing.T) {
	sentinel := errors.New("connection refused")
	src := &PostgresSource{
		reader: &fakeReader{err: sentinel},
	}

	_, _, err := src.Fetch(context.Background(), "abilities.json", "")

	require.Error(t, err)
	require.ErrorIs(t, err, sentinel, "reader errors must propagate (a boot-time load failure), not be swallowed")
}

// A singleton kind reads as a single JSON object via the singleton query path, not the array
// aggregate, and hashes the returned object bytes like every other source.
func TestPostgresSourceFetchSingletonReturnsObject(t *testing.T) {
	obj := []byte(`{"maxPlayers":8,"roundSeconds":90}`)
	fr := &fakeReader{singleton: map[string][]byte{"match_settings": obj}}
	src := &PostgresSource{reader: fr}
	src.RegisterSingleton("match_settings.json")

	got, hash, err := src.Fetch(context.Background(), "match_settings.json", "")

	require.NoError(t, err)
	require.True(t, fr.lastSingle, "a registered singleton must take the single-object read path")
	require.Equal(t, obj, got, "singleton Fetch must return one JSON object, not an array")
	require.Equal(t, sha256Hex(obj), hash)
}

// A kind that was not registered as a singleton must take the array read path.
func TestPostgresSourceFetchNonSingletonReadsArray(t *testing.T) {
	arr := []byte(`[{"id":"a"}]`)
	fr := &fakeReader{rows: map[string][]byte{"abilities": arr}}
	src := &PostgresSource{reader: fr}

	got, _, err := src.Fetch(context.Background(), "abilities.json", "")

	require.NoError(t, err)
	require.False(t, fr.lastSingle, "an unregistered kind must take the array read path")
	require.Equal(t, arr, got)
}

// RegisterKind maps file→table so tableFor returns the registered table; unregistered files fall
// back to the name derived from the file.
func TestPostgresSourceTableFor(t *testing.T) {
	src := &PostgresSource{}

	require.Equal(t, "abilities", src.tableFor("testdata/abilities.json"),
		"an unregistered file must fall back to tableFromFile")

	src.RegisterKind("powerups.json", "powerups_definitions")

	require.Equal(t, "powerups_definitions", src.tableFor("powerups.json"),
		"a registered file must resolve to its explicit table")
	require.Equal(t, "mobs", src.tableFor("data/mobs.json"),
		"other files keep falling back to tableFromFile")
}
